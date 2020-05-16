package noaa

import (
	"testing"
	"time"
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
