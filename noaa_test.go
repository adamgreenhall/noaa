package noaa

import (
	"encoding/json"
	"io/ioutil"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBlank(t *testing.T) {
	point, err := Points("", "")
	if point == nil && err != nil {
		return
	}
	t.Error("noaa.Points() should return a 404 error for a blank lat, lon.")
}

func TestBlankLat(t *testing.T) {
	point, err := Points("", "-147.7390417")
	if point == nil && err != nil {
		return
	}
	t.Error("noaa.Points() should return a 404 error for a blank lat.")
}

func TestBlankLon(t *testing.T) {
	point, err := Points("64.828421", "")
	if point == nil && err != nil {
		return
	}
	t.Error("noaa.Points() should return a 404 error for a blank lon.")
}

func TestZero(t *testing.T) {
	point, err := Points("0", "0")
	if point == nil && err != nil {
		return
	}
	t.Error("noaa.Points() should return a 404 error for a zero lat, lon.")
}

func TestInternational(t *testing.T) {
	point, err := Points("48.85660", "2.3522") // Paris, France
	if point == nil && err != nil {
		return
	}
	t.Error("noaa.Points() should return a 404 error for lat, lon outside the U.S. territories.")
}

func TestAlaska(t *testing.T) {
	point, err := Points("64.828421", "-147.7390417")
	if point != nil && err == nil {
		return
	}
	t.Error("noaa.Points() should return valid points for parts of Alaska.")
}

func TestForecastDuration(t *testing.T) {
	testCases := make(map[string]time.Duration, 0)
	testCases["2019-10-27T09:00:00+00:00/PT1H"] = time.Duration(3600 * 1e9)
	testCases["2019-10-27T09:00:00+00:00/P1DT15H"] = time.Duration((24 + 15) * 3600 * 1e9)
	testCases["2019-10-29T06:00:00+00:00/P5D"] = time.Duration((24 * 5) * 3600 * 1e9)
	for ts, durExpected := range testCases {
		durParsed, err := parseDuration(ts)
		if err != nil {
			t.Error(err)
		}
		if *durParsed != durExpected {
			t.Errorf("computed duration %s doesn't match expected %s", durParsed, durExpected)
		}
	}
}

type TestStruct struct {
	Updated string `json:"updateTime"`
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
	assert.NoError(t, err)
	fcstHourly, err := fcst.Temperature.hourly(fcst.ValidTimes.Time, fcst.ValidTimes.endTime())
	assert.NoError(t, err)
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
		assert.NoError(t, err)
		forecasts[i] = fcst
	}
	fcstAvg, err := AverageForecast(forecasts)
	assert.NoError(t, err)
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
		assert.NoError(t, err)
		forecasts[i] = fcst
	}
	fcstAvg, err := AverageForecast(forecasts)
	assert.NoError(t, err)
	assert.NotNil(t, fcstAvg)
}
