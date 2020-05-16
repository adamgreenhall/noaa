package noaa

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"testing"

	"github.com/stretchr/testify/assert"
)

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func readForecast(file string) (*ForecastGridResponse, error) {
	var forecast ForecastGridResponse
	buf, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(buf, &forecast); err != nil {
		return nil, err
	}
	return &forecast, nil
}

func TestHourly(t *testing.T) {
	fcst, err := readForecast("test_cases/gridForecast1.json")
	check(err)

	tmin := fcst.Temperature.Values[0].Time.Time
	tmax := fcst.Temperature.Values[len(fcst.Temperature.Values)-1].Time.Time
	fmt.Println(fcst.ValidTimes.Duration.Hours(), tmax.Sub(tmin).Hours())
	fmt.Println(tmin.Format(timeFormat))
	fmt.Println(tmax.Format(timeFormat))

	fcstHourly, err := fcst.Temperature.hourly(fcst.ValidTimes.Time, fcst.ValidTimes.endTime())
	check(err)
	assert.Equal(t, int(fcst.ValidTimes.Duration.Hours())+1, len(fcstHourly.Values))
	assert.NotNil(t, fcstHourly.Values[len(fcstHourly.Values)-1].Time)
	assert.NotNil(t, fcstHourly.Values[len(fcstHourly.Values)-1].Value)
	assert.Equal(t, fcst.Temperature.Values[0].Value, fcstHourly.Values[0].Value)
	assert.Equal(t, fcst.Temperature.Values[0].Time.Time, fcstHourly.Values[0].Time.Time)
}

func TestAverage(t *testing.T) {
	var files = [...]string{
		"test_cases/gridForecast1.json",
		"test_cases/gridForecast2.json",
	}
	forecasts := make([]*ForecastGridResponse, len(files))
	for i, file := range files {
		fcst, err := readForecast(file)
		check(err)
		forecasts[i] = fcst
	}
	fcstAvg, err := AverageForecast(forecasts, true)
	check(err)
	assert.NotNil(t, fcstAvg)
	assert.Equal(t, fcstAvg.Temperature.Values[0].Value, (forecasts[0].Temperature.Values[0].Value+forecasts[1].Temperature.Values[0].Value)/2.0)
}

func TestAverageEnd2End(t *testing.T) {
	var endpoints = [...]string{
		"https://api.weather.gov/gridpoints/SEW/151,119",
		"https://api.weather.gov/gridpoints/OTX/37,137",
	}
	forecasts := make([]*ForecastGridResponse, len(endpoints))
	for i, endpoint := range endpoints {
		fcst, err := GetEndpointGridForecast(endpoint)
		check(err)
		forecasts[i] = fcst
	}
	fcstAvg, err := AverageForecast(forecasts, true)
	check(err)
	assert.NotNil(t, fcstAvg)
}
