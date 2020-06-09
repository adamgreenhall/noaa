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
	"strings"
	"time"
)

// Constant values for the weather.gov REST API
const (
	API        = "https://api.weather.gov"
	APIKey     = "github.com/icodealot/noaa" // See auth docs at weather.gov
	APIAccept  = "application/ld+json"       // Changes may affect struct mappings below
	timeFormat = time.RFC3339
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

type forecastElevation struct {
	Value float64 `json:"value"`
	Units string  `json:"unitCode"`
}

// ForecastGridResponse holds the JSON values from /gridpoints/<cwa>/<x,y>
type ForecastGridResponse struct {
	ID                       string              `json:"@id"`
	Updated                  time.Time           `json:"updateTime"`
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

func newForecastGridResponse(updated time.Time, validTimes *ForecastTime, elevation forecastElevation, timeseriesMap map[string]*ForecastTimeseries) (*ForecastGridResponse, error) {
	// TODO: check keys exist
	return &ForecastGridResponse{
		Updated:                  updated,
		ValidTimes:               validTimes,
		Elevation:                elevation,
		Temperature:              timeseriesMap["Temperature"],
		SkyCover:                 timeseriesMap["SkyCover"],
		PrecipitationProbability: timeseriesMap["PrecipitationProbability"],
		PrecipitationQuantity:    timeseriesMap["PrecipitationQuantity"],
		SnowFallAmount:           timeseriesMap["SnowFallAmount"],
		SnowLevel:                timeseriesMap["SnowLevel"],
		WindSpeed:                timeseriesMap["WindSpeed"],
	}, nil
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

// ForecastDetailed returns a set of timeseries in ForecastGridResponse
func ForecastDetailed(lat string, lon string) (*ForecastGridResponse, error) {
	point, err := Points(lat, lon)
	if err != nil {
		return nil, err
	}
	return GetEndpointGridForecast(point.EndpointForecasGrid)
}

// GetEndpointGridForecast returns the forecast for an endpoint
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
