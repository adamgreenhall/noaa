package noaa

import (
	"fmt"
	"time"
)

// AverageForecast takes the mean between many ForecastGridResponse
func AverageForecast(forecasts []*ForecastGridResponse, debug bool) (*ForecastGridResponse, error) {
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
		if debug {
			fmt.Println(fmt.Sprintf("%d %26s %s %s", i, "fcst", fcst.ValidTimes.Time.Format(timeFormat), fcst.ValidTimes.endTime().Format(timeFormat)))
		}
		for k, ts := range fcst.timeseriesMap() {
			timeseriesArrays[k] = append(timeseriesArrays[k], ts)
			if debug {
				fmt.Println(fmt.Sprintf("%d %26s %s %s", i, k, ts.Tmin().Format(timeFormat), ts.Tmax().Format(timeFormat)))
			}
		}
		if fcst.ValidTimes.Time.Before(tsMin) {
			tsMin = fcst.ValidTimes.Time
		}
		if fcst.ValidTimes.endTime().After(tsMax) {
			tsMax = fcst.ValidTimes.endTime()
		}
	}
	if debug {
		fmt.Println(fmt.Sprintf("%28s %s %s", "result", tsMin.Format(timeFormat), tsMax.Format(timeFormat)))
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

func (ts *ForecastTimeseries) hourly(tMin, tMax time.Time) (*ForecastTimeseries, error) {
	Nhours := int(tMax.Sub(tMin).Hours()) + 1
	tsTmin := ts.Tmin()
	tsTmax := ts.Tmax()
	lenTs := len(ts.Values)

	out := make([]*ForecastTimeseriesValue, Nhours)
	hr := 0
	firstValue := ts.Values[0]
	lastValue := ts.Values[len(ts.Values)-1]
	padHoursStart := int(tsTmin.Sub(tMin).Hours())
	for i := 0; i < padHoursStart; i++ {
		tNew := tMin.Add(time.Duration(i) * time.Hour)
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
				continue
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
	firstHourlyValue := out[0]
	lastHourlyValue := out[len(out)-1]
	msgDebugging := (fmt.Sprintf("original: len=%03d, tmin=%s, tmax=%s\n", lenTs, tsTmin.Format(timeFormat), tsTmax.Format(timeFormat)) +
		fmt.Sprintf("hourly  : len=%03d, tmin=%s, tmax=%s", Nhours, tMin.Format(timeFormat), tMax.Format(timeFormat)))
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

func averageForecastTimeseries(key string, forecasts []*ForecastTimeseries, tsMin time.Time, tsMax time.Time, rootForecasts []*ForecastGridResponse) (*ForecastTimeseries, error) {
	N := float64(len(forecasts))
	fcstBase, err := forecasts[0].hourly(tsMin, tsMax)
	if err != nil {
		return nil, fmt.Errorf("failed to convert forecast[0]=%s to hourly.\n%s", rootForecasts[0].ID, err.Error())
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
