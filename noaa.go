// Package noaa implements a basic wrapper around api.weather.gov to
// grab HTTP responses to endpoints (i.e.: weather & forecast data)
// by the National Weather Service, an agency of the United States.
package noaa

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Constant values for the weather.gov REST API
const (
	API       = "https://api.weather.gov"
	APIKey    = "github.com/icodealot/noaa" // See auth docs at weather.gov
	APIAccept = "application/ld+json"       // Changes may affect struct mappings below
)

// PointsResponse holds the JSON values from /points/<lat,lon>
type PointsResponse struct {
	ID                          string `json:"@id"`
	CWA                         string `json:"cwa"`
	Office                      string `json:"forecastOffice"`
	GridX                       int64  `json:"gridX"`
	GridY                       int64  `json:"gridY"`
	EndpointForecast            string `json:"forecast"`
	EndpointForecastHourly      string `json:"forecastHourly"`
	EndpointForecasGrid         string `json:"forecastGridData"`
	EndpointObservationStations string `json:"observationStations"`
	Timezone                    string `json:"timeZone"`
	RadarStation                string `json:"radarStation"`
}

// StationsResponse holds the JSON values from /points/<lat,lon>/stations
type StationsResponse struct {
	Stations []string `json:"observationStations"`
}

// ForecastResponse holds the JSON values from /gridpoints/<cwa>/<x,y>/forecast"
type ForecastResponse struct {
	// capture data from the forecast
	Updated   string `json:"updated"`
	Units     string `json:"units"`
	Elevation struct {
		Value float64 `json:"value"`
		Units string  `json:"unitCode"`
	} `json:"elevation"`
	Periods []struct {
		ID              int32   `json:"number"`
		Name            string  `json:"name"`
		StartTime       string  `json:"startTime"`
		EndTime         string  `json:"endTime"`
		IsDaytime       bool    `json:"isDaytime"`
		Temperature     float64 `json:"temperature"`
		TemperatureUnit string  `json:"temperatureUnit"`
		WindSpeed       string  `json:"windSpeed"`
		WindDirection   string  `json:"windDirection"`
		Summary         string  `json:"shortForecast"`
		Details         string  `json:"detailedForecast"`
	} `json:"periods"`
	Point *PointsResponse
}

// ForecastTimeseriesValue is one timepoint of a forecast timeseries
type ForecastTimeseriesValue struct {
	Time  ForecastTime `json:"validTime,string"`
	Value float64      `json:"value"`
}

// ForecastTimeseries holds the hourly forecasts values from within ForecastGridResponse
type ForecastTimeseries struct {
	Units  string                     `json:"uom"`
	Values []*ForecastTimeseriesValue `json:"values"`
}

type forecastElevation struct {
	Value float64 `json:"value"`
	Units string  `json:"unitCode"`
}

type forecastGridResponseRaw struct {
	Updated                  string              `json:"updateTime"`
	Elevation                forecastElevation   `json:"elevation"`
	Temperature              *ForecastTimeseries `json:"temperature"`
	SkyCover                 *ForecastTimeseries `json:"skyCover"`
	WindSpeed                *ForecastTimeseries `json:"windSpeed"`
	PrecipitationProbability *ForecastTimeseries `json:"probabilityOfPrecipitation"`
	PrecipitationQuantity    *ForecastTimeseries `json:"quantitativePrecipitation"`
	SnowFallAmount           *ForecastTimeseries `json:"snowfallAmount"`
	SnowLevel                *ForecastTimeseries `json:"snowLevel"`
}

// ForecastGridResponse holds the JSON values from /gridpoints/<cwa>/<x,y>
type ForecastGridResponse struct {
	Updated    string `json:"updateTime"`
	Point      *PointsResponse
	Elevation  forecastElevation              `json:"elevation"`
	Timeseries map[string]*ForecastTimeseries `json:"timeseries"`
}

// AverageForecast takes the mean between many ForecastGridResponse
func AverageForecast(forecasts []*ForecastGridResponse) (*ForecastGridResponse, error) {
	if len(forecasts) == 0 {
		return nil, fmt.Errorf("no forecasts to average")
	}
	N := float64(len(forecasts))
	baseElevationUnits := forecasts[0].Elevation.Units
	meanElevation := 0.0
	timeseriesArrays := make(map[string][]*ForecastTimeseries, 0)
	meanTimeseries := make(map[string]*ForecastTimeseries, 0)
	for i, fcst := range forecasts {
		if fcst.Elevation.Units != baseElevationUnits {
			return nil, fmt.Errorf("elevation units must match. units[i=%d] %s != %s", i, fcst.Elevation.Units, baseElevationUnits)
		}
		meanElevation += fcst.Elevation.Value / N
		for k, ts := range fcst.Timeseries {
			timeseriesArrays[k] = append(timeseriesArrays[k], ts)
		}
	}
	for k, ts := range timeseriesArrays {
		tsMean, err := averageForecastTimeseries(ts)
		if err != nil {
			return nil, err
		}
		meanTimeseries[k] = tsMean
	}
	return &ForecastGridResponse{
		Updated: forecasts[0].Updated,
		Elevation: forecastElevation{
			Value: meanElevation,
			Units: forecasts[0].Elevation.Units,
		},
		Timeseries: meanTimeseries,
	}, nil
}

func averageForecastTimeseries(forecasts []*ForecastTimeseries) (*ForecastTimeseries, error) {
	N := float64(len(forecasts))
	baseUnits := forecasts[0].Units
	avgValues := make([]*ForecastTimeseriesValue, 0)
	for _, elem := range forecasts[0].Values {
		avgValues = append(avgValues, &ForecastTimeseriesValue{Time: elem.Time, Value: 0.0})
	}
	for i, fcst := range forecasts {
		if fcst.Units != baseUnits {
			return nil, fmt.Errorf("units must match units[i=%d] %s != %s", i, fcst.Units, baseUnits)
		}
		if len(fcst.Values) != len(avgValues) {
			return nil, fmt.Errorf("timeseries length must match. lenght[i=%d] of %d != %d", i, len(fcst.Values), len(avgValues))
		}
		for e, elem := range fcst.Values {
			if elem.Time != avgValues[e].Time {
				return nil, fmt.Errorf("times must match. time[i%d] of %s != %s", e, elem.Time, avgValues[e].Time)
			}
			avgValues[e].Value += elem.Value / N
		}
	}
	return &ForecastTimeseries{Units: baseUnits, Values: avgValues}, nil
}

// Cache used for point lookup to save some HTTP round trips
// key is expected to be PointsResponse.ID
var pointsCache = map[string]*PointsResponse{}

// Call the weather.gov API. We could just use http.Get() but
// since we need to include some custom header values this helps.
func apiCall(endpoint string) (res *http.Response, err error) {
	endpoint = strings.Replace(endpoint, "http://", "https://", -1)
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Accept", APIAccept)
	req.Header.Add("User-Agent", APIKey) // See http://www.weather.gov/documentation/services-web-api

	res, err = http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	if res.StatusCode == 404 {
		defer res.Body.Close()
		return nil, errors.New("404: data not found for -> " + endpoint)
	}
	if res.StatusCode != 200 {
		defer res.Body.Close()
		return nil, fmt.Errorf("%d: data not found for -> %s", res.StatusCode, endpoint)
	}
	return res, nil
}

// Points returns a set of useful endpoints for a given <lat,lon>
// or returns a cached object if appropriate
func Points(lat string, lon string) (points *PointsResponse, err error) {
	endpoint := fmt.Sprintf("%s/points/%s,%s", API, lat, lon)
	if pointsCache[endpoint] != nil {
		return pointsCache[endpoint], nil
	}
	res, err := apiCall(endpoint)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	decoder := json.NewDecoder(res.Body)
	if err = decoder.Decode(&points); err != nil {
		return nil, err
	}
	pointsCache[endpoint] = points
	return points, nil
}

// Stations returns an array of observation station IDs (urls)
func Stations(lat string, lon string) (stations *StationsResponse, err error) {
	point, err := Points(lat, lon)
	if err != nil {
		return nil, err
	}
	res, err := apiCall(point.EndpointObservationStations)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	decoder := json.NewDecoder(res.Body)
	if err = decoder.Decode(&stations); err != nil {
		return nil, err
	}
	return stations, nil
}

// Forecast returns an array of forecast observations (14 periods and 2/day max)
func Forecast(lat string, lon string) (forecast *ForecastResponse, err error) {
	point, err := Points(lat, lon)
	if err != nil {
		return nil, err
	}
	res, err := apiCall(point.EndpointForecast)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	decoder := json.NewDecoder(res.Body)
	if err = decoder.Decode(&forecast); err != nil {
		return nil, err
	}
	forecast.Point = point
	return forecast, nil
}

// ForecastTime parses the NWS time format
type ForecastTime struct {
	Time     time.Time
	Duration time.Duration
}

func parseDuration(t string) (*time.Duration, error) {
	durationRegex := regexp.MustCompile(`([0-9]d)?(t[0-9]+h)?`)
	if !strings.Contains(t, "P") {
		return nil, fmt.Errorf("no duration suffix found for time %s", t)
	}
	durStr := strings.ToLower(strings.Split(t, "P")[1])
	matches := durationRegex.FindStringSubmatch(durStr)
	dur := time.Duration(0)
	if len(matches[1]) > 0 {
		durIntDays, err := strconv.Atoi(strings.ReplaceAll(matches[1], "d", ""))
		if err != nil {
			return nil, err
		}
		durDays, err := time.ParseDuration(fmt.Sprintf("%dh", durIntDays*24))
		if err != nil {
			return nil, err
		}
		dur += durDays
	}
	if len(matches[2]) > 0 {
		durHours, err := time.ParseDuration(strings.ReplaceAll(matches[2], "t", ""))
		if err != nil {
			return nil, err
		}
		dur += durHours
	}
	return &dur, nil
}

// UnmarshalJSON parses the NWS time format
func (t *ForecastTime) UnmarshalJSON(buf []byte) error {
	ttStr := strings.ReplaceAll(string(buf), `"`, "")
	tBase := strings.Split(ttStr, "+")[0]
	tt, err := time.Parse(time.RFC3339, fmt.Sprintf("%sZ", tBase))
	if err != nil {
		return err
	}
	dur, err := parseDuration(ttStr)
	if err != nil {
		return err
	}
	t.Time = tt
	t.Duration = *dur
	return nil
}

// ForecastDetailed returns a set of timeseries in ForecastGridResponse
func ForecastDetailed(lat string, lon string) (*ForecastGridResponse, error) {
	point, err := Points(lat, lon)
	if err != nil {
		return nil, err
	}
	res, err := apiCall(point.EndpointForecasGrid)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	var forecastRaw forecastGridResponseRaw
	if err = json.Unmarshal(body, &forecastRaw); err != nil {
		return nil, err
	}
	timeseries := make(map[string]*ForecastTimeseries, 0)
	timeseries["Temperature"] = forecastRaw.Temperature
	timeseries["SkyCover"] = forecastRaw.SkyCover
	timeseries["WindSpeed"] = forecastRaw.WindSpeed
	timeseries["PrecipitationProbability"] = forecastRaw.PrecipitationProbability
	timeseries["PrecipitationQuantity"] = forecastRaw.PrecipitationQuantity
	timeseries["SnowFallAmount"] = forecastRaw.SnowFallAmount
	timeseries["SnowLevel"] = forecastRaw.SnowLevel
	return &ForecastGridResponse{
		Updated:    forecastRaw.Updated,
		Point:      point,
		Elevation:  forecastRaw.Elevation,
		Timeseries: timeseries,
	}, nil
}
