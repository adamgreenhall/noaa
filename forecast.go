package noaa

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ForecastTime parses the NWS time format
type ForecastTime struct {
	Time     time.Time
	Duration time.Duration
}

// ForecastTimeseries holds the hourly forecasts values from within ForecastGridResponse
type ForecastTimeseries struct {
	Name   string
	ID     string
	Units  string                     `json:"uom"`
	Values []*ForecastTimeseriesValue `json:"values"`
}

// ForecastTimeseriesValue is one timepoint of a forecast timeseries
type ForecastTimeseriesValue struct {
	Time  ForecastTime `json:"validTime,string"`
	Value float64      `json:"value"`
}

// Tmin is the minimum time
func (ts *ForecastTimeseries) Tmin() time.Time {
	return ts.Values[0].Time.Time
}

// Tmax is the max time
func (ts *ForecastTimeseries) Tmax() time.Time {
	return ts.Values[len(ts.Values)-1].Time.endTime()
}

func (ts *ForecastTimeseries) fillInfo(name string, id string) *ForecastTimeseries {
	ts.Name = name
	ts.ID = id
	return ts
}

func (t *ForecastTime) endTime() time.Time {
	return t.Time.Add(t.Duration)
}

func parseDuration(t string) (*time.Duration, error) {
	durationRegex := regexp.MustCompile(`([0-9]d)?t?([0-9]+h)?([0-9]+m)?`)
	if !strings.Contains(t, "P") {
		return nil, fmt.Errorf("no duration suffix found for time %s", t)
	}
	durStr := strings.ToLower(strings.Split(t, "P")[1])
	matches := durationRegex.FindStringSubmatch(durStr)
	if len(matches) == 0 {
		return nil, fmt.Errorf("duration pattern does not match expected: %s", t)
	}
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
	if len(matches[3]) > 0 {
		durMinutes, err := time.ParseDuration(matches[3])
		if err != nil {
			return nil, err
		}
		// round up to the next hour
		if durMinutes >= 1*time.Minute {
			dur += time.Hour
		}
	}
	return &dur, nil
}

// UnmarshalJSON parses the NWS time format
func (t *ForecastTime) UnmarshalJSON(buf []byte) error {
	ttStr := strings.ReplaceAll(string(buf), `"`, "")
	tBase := strings.Split(ttStr, "+")[0]
	// truncate to hour
	tBase = strings.Split(tBase, ":")[0] + ":00:00Z"
	tt, err := time.Parse(time.RFC3339, tBase)
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
