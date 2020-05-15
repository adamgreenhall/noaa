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
	Name   string
	ID     string
	Units  string                     `json:"uom"`
	Values []*ForecastTimeseriesValue `json:"values"`
}

func (ts *ForecastTimeseries) fillInfo(name string, id string) *ForecastTimeseries {
	ts.Name = name
	ts.ID = id
	return ts
}

type forecastElevation struct {
	Value float64 `json:"value"`
	Units string  `json:"unitCode"`
}

// ForecastGridResponse holds the JSON values from /gridpoints/<cwa>/<x,y>
type ForecastGridResponse struct {
	ID                       string              `json:"@id"`
	Updated                  string              `json:"updateTime"`
	ValidTimes               *ForecastTime       `json:"validTimes"`
	Elevation                forecastElevation   `json:"elevation"`
	Temperature              *ForecastTimeseries `json:"temperature"`
	SkyCover                 *ForecastTimeseries `json:"skyCover"`
	WindSpeed                *ForecastTimeseries `json:"windSpeed"`
	PrecipitationProbability *ForecastTimeseries `json:"probabilityOfPrecipitation"`
	PrecipitationQuantity    *ForecastTimeseries `json:"quantitativePrecipitation"`
	SnowFallAmount           *ForecastTimeseries `json:"snowfallAmount"`
	SnowLevel                *ForecastTimeseries `json:"snowLevel"`
}

func (f *ForecastGridResponse) timeseriesMap() map[string]*ForecastTimeseries {
	timeseries := make(map[string]*ForecastTimeseries, 0)
	timeseries["Temperature"] = f.Temperature.fillInfo("Temperature", f.ID)
	timeseries["SkyCover"] = f.SkyCover.fillInfo("SkyCover", f.ID)
	timeseries["WindSpeed"] = f.WindSpeed.fillInfo("WindSpeed", f.ID)
	timeseries["PrecipitationProbability"] = f.PrecipitationProbability.fillInfo("PrecipitationProbability", f.ID)
	timeseries["PrecipitationQuantity"] = f.PrecipitationQuantity.fillInfo("PrecipitationQuantity", f.ID)
	timeseries["SnowFallAmount"] = f.SnowFallAmount.fillInfo("SnowFallAmount", f.ID)
	timeseries["SnowLevel"] = f.SnowLevel.fillInfo("SnowLevel", f.ID)
	return timeseries
}

func newForecastGridResponse(updated string, elevation forecastElevation, timeseriesMap map[string]*ForecastTimeseries) (*ForecastGridResponse, error) {
	// TODO: check keys exist
	return &ForecastGridResponse{
		Updated:                  updated,
		Elevation:                elevation,
		Temperature:              timeseriesMap["Temperature"],
		SkyCover:                 timeseriesMap["SkyCover"],
		PrecipitationProbability: timeseriesMap["PrecipitationProbability"],
		PrecipitationQuantity:    timeseriesMap["PrecipitationQuantity"],
		SnowFallAmount:           timeseriesMap["SnowFallAmount"],
		SnowLevel:                timeseriesMap["SnowLevel"],
	}, nil
}

// AverageForecast takes the mean between many ForecastGridResponse
func AverageForecast(forecasts []*ForecastGridResponse) (*ForecastGridResponse, error) {
	if len(forecasts) == 0 {
		return nil, fmt.Errorf("no forecasts to average")
	}
	N := float64(len(forecasts))
	tsMin := forecasts[0].Temperature.Values[0].Time.Time
	tsMax := forecasts[0].Temperature.Values[0].Time.Time
	baseElevationUnits := forecasts[0].Elevation.Units
	meanElevation := 0.0
	timeseriesArrays := make(map[string][]*ForecastTimeseries, 0)
	meanTimeseries := make(map[string]*ForecastTimeseries, 0)
	for i, fcst := range forecasts {
		if fcst.Elevation.Units != baseElevationUnits {
			return nil, fmt.Errorf("elevation units must match. units[i=%d] %s != %s", i, fcst.Elevation.Units, baseElevationUnits)
		}
		meanElevation += fcst.Elevation.Value / N
		for k, ts := range fcst.timeseriesMap() {
			timeseriesArrays[k] = append(timeseriesArrays[k], ts)
		}
		if fcst.ValidTimes.Time.Before(tsMin) {
			tsMin = fcst.ValidTimes.Time
		}
		if fcst.ValidTimes.endTime().After(tsMax) {
			tsMax = fcst.ValidTimes.endTime()
		}
	}
	for k, ts := range timeseriesArrays {
		tsMean, err := averageForecastTimeseries(k, ts, tsMin, tsMax, forecasts)
		if err != nil {
			return nil, err
		}
		meanTimeseries[k] = tsMean
	}
	return newForecastGridResponse(
		forecasts[0].Updated,
		forecastElevation{
			Value: meanElevation,
			Units: forecasts[0].Elevation.Units,
		},
		meanTimeseries,
	)
}

func (ts *ForecastTimeseries) hourly(tsMin time.Time, tsMax time.Time) (*ForecastTimeseries, error) {
	Nhours := int(tsMax.Sub(tsMin).Hours()) + 1
	out := make([]*ForecastTimeseriesValue, Nhours)
	hr := 0
	firstValue := ts.Values[0]
	padHoursStart := int(firstValue.Time.Time.Sub(tsMin).Hours())
	for i := 0; i < padHoursStart; i++ {
		tNew := tsMin.Add(time.Duration(i) * time.Hour)
		out[hr] = &ForecastTimeseriesValue{
			Time: ForecastTime{
				Time:     tNew,
				Duration: time.Hour,
			},
			Value: firstValue.Value,
		}
		hr++
	}
	for _, t := range ts.Values {
		for i := 0; i < int(t.Time.Duration.Hours()); i++ {
			tNew := t.Time.Time.Add(time.Duration(i) * time.Hour)
			if hr >= Nhours {
				return nil, fmt.Errorf("attempting to extend hourly forecast for %s beyond bounds. length=%d, tmin=%s, tNew=%s, tmax=%s", ts.Name, Nhours, tsMin, tNew, tsMax)
			}
			out[hr] = &ForecastTimeseriesValue{
				Time: ForecastTime{
					Time:     tNew,
					Duration: time.Hour,
				},
				Value: t.Value,
			}
			hr++
		}
	}
	// fill values at end of timeseries
	lastValue := out[hr-1]
	padHoursEnd := Nhours - hr
	for i := 1; i <= padHoursEnd; i++ {
		out[hr] = &ForecastTimeseriesValue{
			Time: ForecastTime{
				Time:     lastValue.Time.Time.Add(time.Duration(i) * time.Hour),
				Duration: time.Hour,
			},
			Value: lastValue.Value,
		}
		hr++
	}
	firstValue = out[0]
	lastValue = out[len(out)-1]
	if firstValue.Time.Time != tsMin {
		return nil, fmt.Errorf("start times do not match for %s at %s.\nexpected=%s\nfound=   %s", ts.Name, ts.ID, tsMin, firstValue.Time.Time)
	}
	if lastValue.Time.Time != tsMax {
		return nil, fmt.Errorf("end times do not match for %s at %s.\nexpected=%s\nfound=   %s", ts.Name, ts.ID, tsMax, lastValue.Time.Time)
	}
	return &ForecastTimeseries{
		Name:   ts.Name,
		ID:     ts.ID,
		Values: out,
		Units:  ts.Units,
	}, nil
}

func averageForecastTimeseries(key string, forecasts []*ForecastTimeseries, tsMin time.Time, tsMax time.Time, rootForecasts []*ForecastGridResponse) (*ForecastTimeseries, error) {
	N := float64(len(forecasts))
	fcstBase, err := forecasts[0].hourly(tsMin, tsMax)
	if err != nil {
		return nil, fmt.Errorf("failed to convert forecast[0]=%s to hourly. %s", rootForecasts[0].ID, err.Error())
	}
	baseUnits := fcstBase.Units
	avgValues := make([]*ForecastTimeseriesValue, len(fcstBase.Values))
	// convert each of these ts to hourly timeseries (currently irregular)
	for i, elem := range fcstBase.Values {
		avgValues[i] = &ForecastTimeseriesValue{Time: elem.Time, Value: 0.0}
	}
	for i, fcst := range forecasts {
		if fcst.Units != baseUnits {
			return nil, fmt.Errorf("units must match units[i=%d] %s != %s", i, fcst.Units, baseUnits)
		}
		fcstHourly, err := fcst.hourly(tsMin, tsMax)
		if err != nil {
			return nil, fmt.Errorf("failed to convert forecast[%d]=%s to hourly. %s", i, rootForecasts[i].ID, err.Error())
		}
		if len(fcstHourly.Values) != len(avgValues) {
			return nil, fmt.Errorf(
				"timeseries length must match for %s. lenght[i=%d] of %d != %d. Forecast endpoints for %s:\n%s\n%s",
				key,
				i,
				len(fcstHourly.Values), len(avgValues),
				key,
				rootForecasts[0].ID,
				rootForecasts[i].ID,
			)
		}
		for e, elem := range fcstHourly.Values {
			if elem.Time != avgValues[e].Time {
				return nil, fmt.Errorf(
					"times must match. time[i%d] of %s != %s. tsMin=%s. Forecast endpoints for %s:\n%s\n%s",
					e,
					elem.Time,
					avgValues[e].Time,
					tsMin,
					key,
					rootForecasts[0].ID,
					rootForecasts[i].ID,
				)
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

func (t *ForecastTime) endTime() time.Time {
	return t.Time.Add(t.Duration)
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
	return GetEndpointGridForecast(point.EndpointForecasGrid)
}

// GetEndpointGridForecast gets the forceast for an endpoint
func GetEndpointGridForecast(endpoint string) (*ForecastGridResponse, error) {
	res, err := apiCall(endpoint)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	var forecast ForecastGridResponse
	if err = json.Unmarshal(body, &forecast); err != nil {
		return nil, err
	}
	forecast.ID = endpoint
	return &forecast, nil
}
