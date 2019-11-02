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

func pointResponseMock() *PointsResponse {
	return &PointsResponse{
		ID:                          "123",
		CWA:                         "abc",
		Office:                      "off",
		GridX:                       128,
		GridY:                       33,
		EndpointForecast:            "http://endpoint",
		EndpointForecastHourly:      "http://endpoint",
		EndpointForecasGrid:         "http://endpoint",
		EndpointObservationStations: "stations",
		Timezone:                    "tz",
		RadarStation:                "station",
	}
}

func TestHourly(t *testing.T) {
	var forecastRaw forecastGridResponseRaw
	point := pointResponseMock()
	buf, err := ioutil.ReadFile("test_cases/gridForecast1.json")
	assert.NoError(t, err)
	err = json.Unmarshal(buf, &forecastRaw)
	assert.NoError(t, err)

	fcst, err := newForecastGridResponse(&forecastRaw, point)
	assert.NoError(t, err)
	fcstHourly, err := fcst.Timeseries["Temperature"].hourly(fcst.ValidTimes.Time, fcst.ValidTimes.endTime())
	assert.NoError(t, err)
	assert.Equal(t, int(fcst.ValidTimes.Duration.Hours()), len(fcstHourly.Values))
	assert.NotNil(t, fcstHourly.Values[len(fcstHourly.Values)-1].Time)
	assert.NotNil(t, fcstHourly.Values[len(fcstHourly.Values)-1].Value)
}

func TestAverage(t *testing.T) {
	point := pointResponseMock()
	var files = [...]string{
		"test_cases/gridForecast1.json",
		"test_cases/gridForecast2.json",
	}
	forecasts := make([]*ForecastGridResponse, len(files))
	for i, file := range files {
		var forecastRaw forecastGridResponseRaw
		buf, err := ioutil.ReadFile(file)
		assert.NoError(t, err)
		err = json.Unmarshal(buf, &forecastRaw)
		assert.NoError(t, err)
		fcst, err := newForecastGridResponse(&forecastRaw, point)
		assert.NoError(t, err)
		forecasts[i] = fcst
	}
	fcstAvg, err := AverageForecast(forecasts)
	assert.NoError(t, err)
	assert.NotNil(t, fcstAvg)
	assert.Equal(t, fcstAvg.Timeseries["Temperature"].Values[0].Value, (forecasts[0].Timeseries["Temperature"].Values[0].Value+forecasts[1].Timeseries["Temperature"].Values[0].Value)/2.0)
}
