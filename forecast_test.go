package noaa

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParseDuration(t *testing.T) {
	var durations = [...]string{
		"PT3H",
		"P1DT2H",
		"PT2H59M40S",
		"P5DT10H14M34S",
	}
	var durationTimes = [...]time.Duration{
		time.Hour * 3,
		time.Hour * 26,
		time.Hour * 3,
		time.Hour * (5*24 + 11),
	}
	for i, dur := range durations {
		td, err := parseDuration(dur)
		check(err)
		assert.Equal(t, *td, durationTimes[i])
	}
}

func TestParseTime(t *testing.T) {
	// truncate to hour
	var timeStrings = [...]string{
		"2020-08-19T04:00:00+00:00/PT5H",
		"2020-08-19T09:43:26+00:00/PT6H16M34S",
	}
	var times = make([]time.Time, len(timeStrings))
	ts, err := time.Parse(time.RFC3339, "2020-08-19T04:00:00Z")
	times[0] = ts
	check(err)
	ts, err = time.Parse(time.RFC3339, "2020-08-19T09:00:00Z")
	times[1] = ts
	var durations = [...]time.Duration{
		time.Hour * 5,
		time.Hour * 7,
	}
	for i, ts := range timeStrings {
		var ft ForecastTimeseriesValue
		check(json.Unmarshal([]byte(fmt.Sprintf(`{"validTime": "%s"}`, ts)), &ft))
		assert.Equal(t, ft.Time.Time, times[i])
		assert.Equal(t, ft.Time.Duration, durations[i])
	}
}
