package slo

import "time"

type BurnRateWindow struct {
	Severity    string
	BurnRate    float64
	LongWindow  time.Duration
	ShortWindow time.Duration
	For         time.Duration
}

func DefaultBurnRateWindows() []BurnRateWindow {
	return []BurnRateWindow{
		{
			Severity:    "critical",
			BurnRate:    14.4,
			LongWindow:  1 * time.Hour,
			ShortWindow: 5 * time.Minute,
			For:         2 * time.Minute,
		},
		{
			Severity:    "critical",
			BurnRate:    6.0,
			LongWindow:  6 * time.Hour,
			ShortWindow: 30 * time.Minute,
			For:         5 * time.Minute,
		},
		{
			Severity:    "warning",
			BurnRate:    3.0,
			LongWindow:  24 * time.Hour,
			ShortWindow: 2 * time.Hour,
			For:         10 * time.Minute,
		},
		{
			Severity:    "warning",
			BurnRate:    1.0,
			LongWindow:  3 * 24 * time.Hour,
			ShortWindow: 6 * time.Hour,
			For:         30 * time.Minute,
		},
	}
}

func ScaleWindowsForPeriod(base []BurnRateWindow, period time.Duration) []BurnRateWindow {
	thirtyDays := 30 * 24 * time.Hour
	ratio := float64(period) / float64(thirtyDays)

	scaled := make([]BurnRateWindow, len(base))
	for i, w := range base {
		scaled[i] = BurnRateWindow{
			Severity:    w.Severity,
			BurnRate:    w.BurnRate,
			LongWindow:  scaleDuration(w.LongWindow, ratio, 5*time.Minute),
			ShortWindow: scaleDuration(w.ShortWindow, ratio, 1*time.Minute),
			For:         w.For,
		}
	}
	return scaled
}

func scaleDuration(d time.Duration, ratio float64, min time.Duration) time.Duration {
	scaled := time.Duration(float64(d) * ratio)
	if scaled < min {
		return min
	}
	return scaled
}
