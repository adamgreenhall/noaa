package noaa

import (
	"fmt"
	"math"
	"strings"
	"time"
)

var seriesNames = []string{
	"Temperature",
	"SkyCover",
	"WindSpeed",
	"PrecipitationProbability",
	"PrecipitationQuantity",
	"SnowFallAmount",
	"SnowLevel",
}

// ForecastHourly is a more compact form of noaa.ForecastGridResponse
type ForecastHourly struct {
	CreatedAt       time.Time   `json:"createdAt"`
	ElevationMeters int64       `json:"elevationMeters"`
	Endpoint        string      `json:"endpoint"`
	Times           []time.Time `json:"times"`
	SeriesNames     []string    `json:"seriesNames"`
	Units           []string    `json:"units"`
	Values          [][]float64 `json:"values"`
}

func (ts *ForecastTimeseries) hourly(tMin, tMax time.Time) (*ForecastTimeseries, error) {
	Nhours := int(tMax.Sub(tMin).Hours()) + 1
	tsTmin := ts.Tmin()
	tsTmax := ts.Tmax()
	lenTs := len(ts.Values)
	msgDebugging := (fmt.Sprintf("original series: len=%03d, tmin=%s, tmax=%s\n", lenTs, tsTmin.Format(timeFormat), tsTmax.Format(timeFormat)) +
		fmt.Sprintf("hourly series  : len=%03d, tmin=%s, tmax=%s", Nhours, tMin.Format(timeFormat), tMax.Format(timeFormat)))

	out := make([]*ForecastTimeseriesValue, Nhours)
	hr := 0
	firstValueSeries := ts.Values[0]
	padHoursStart := int(tsTmin.Sub(tMin).Hours())
	for i := 0; i < padHoursStart; i++ {
		out[hr] = &ForecastTimeseriesValue{
			Time: ForecastTime{
				Time:     tMin.Add(time.Duration(i) * time.Hour),
				Duration: time.Hour,
			},
			Value: firstValueSeries.Value,
		}
		hr++
	}
	for _, t := range ts.Values {
		if hr > 0 && hr < Nhours {
			// subsequent values in the original time series sometimes have gaps - interpolate using last value
			lastValue := out[hr-1]
			hrsInterpolate := int(t.Time.Time.Sub(lastValue.Time.Time).Hours())
			for j := 1; j < hrsInterpolate; j++ {
				out[hr] = &ForecastTimeseriesValue{
					Time: ForecastTime{
						Time:     lastValue.Time.Time.Add(time.Duration(j) * time.Hour),
						Duration: time.Hour,
					},
					Value: lastValue.Value,
				}
				hr++
			}
		}
		hrsSpan := int(t.Time.Duration.Hours())
		for i := 0; i < hrsSpan; i++ {
			if hr >= Nhours {
				// reached the end of the hourly series
				// the original series may have extra data (exists outside of the forecast's valid range),
				// but we're cutting it off
				break
			}
			out[hr] = &ForecastTimeseriesValue{
				Time: ForecastTime{
					Time:     t.Time.Time.Add(time.Duration(i) * time.Hour),
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
		out[hr+i-1] = &ForecastTimeseriesValue{
			Time: ForecastTime{
				Time:     lastValue.Time.Time.Add(time.Duration(i) * time.Hour),
				Duration: time.Hour,
			},
			Value: lastValue.Value,
		}
	}
	firstHourlyValue := out[0]
	lastHourlyValue := out[Nhours-1]
	if firstHourlyValue.Time.Time != tMin {
		return nil, fmt.Errorf(
			"start times do not match for %s at %s.\nexpected=%s\nfound=   %s\n%s",
			ts.Name, ts.ID,
			tMin.Format(timeFormat), firstHourlyValue.Time.Time.Format(timeFormat),
			msgDebugging,
		)
	}
	if lastHourlyValue.Time.Time != tMax {
		return nil, fmt.Errorf(
			"end times do not match for %s at %s.\nexpected=%s\nfound=   %s\n%s",
			ts.Name, ts.ID,
			tMax.Format(timeFormat), lastHourlyValue.Time.Time.Format(timeFormat),
			msgDebugging,
		)
	}
	return &ForecastTimeseries{
		Name:   ts.Name,
		ID:     ts.ID,
		Values: out,
		Units:  ts.Units,
	}, nil
}

// CreateForecastHourly builds a ForecastHourly from noaa.ForecastGridResponse
func CreateForecastHourly(grid *ForecastGridResponse) (*ForecastHourly, error) {
	hourlyTimeseries := make(map[string]*ForecastTimeseries)
	for k, ts := range grid.timeseriesMap() {
		ts, err := ts.hourly(grid.ValidTimes.Time, grid.ValidTimes.endTime())
		if err != nil {
			return nil, err
		}
		hourlyTimeseries[k] = ts
	}
	times := make([]time.Time, len(hourlyTimeseries["Temperature"].Values))
	for i, val := range hourlyTimeseries["Temperature"].Values {
		times[i] = val.Time.Time
	}
	values := make([][]float64, len(seriesNames))
	units := make([]string, len(seriesNames))
	for i, nm := range seriesNames {
		ts, ok := hourlyTimeseries[nm]
		if !ok {
			return nil, fmt.Errorf("could not find series %s", nm)
		}
		units[i] = strings.ReplaceAll(
			strings.ReplaceAll(
				strings.ReplaceAll(ts.Units, "unit:", ""),
				"m_s-1", "m/s"),
			"degC",
			"C")
		values[i] = make([]float64, len(times))
		for j, val := range ts.Values {
			// round to one digit precision
			values[i][j] = math.Round(val.Value*10) / 10
		}
	}
	if !strings.HasSuffix(strings.ToLower(grid.Elevation.Units), "unit:m") {
		return nil, fmt.Errorf("unknown elevation units: %s", grid.Elevation.Units)
	}
	return &ForecastHourly{
		CreatedAt:       grid.Updated,
		ElevationMeters: int64(grid.Elevation.Value),
		Endpoint:        grid.ID,
		Times:           times,
		SeriesNames:     seriesNames,
		Units:           units,
		Values:          values,
	}, nil
}
