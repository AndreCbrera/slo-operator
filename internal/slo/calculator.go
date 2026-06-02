package slo

import (
	"fmt"
	"strconv"
	"time"

	v1alpha1 "github.com/AndreCbrera/slo-operator/api/v1alpha1"
)

func MaxErrorRate(target float64) float64 {
	return 1.0 - (target / 100.0)
}

func ErrorBudgetSeconds(target float64, window time.Duration) float64 {
	return MaxErrorRate(target) * window.Seconds()
}

func BurnRate(actualErrorRate, maxErrorRate float64) float64 {
	if maxErrorRate == 0 {
		return 0
	}
	return actualErrorRate / maxErrorRate
}

func ParseTarget(target string) (float64, error) {
	v, err := strconv.ParseFloat(target, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid target %q: %w", target, err)
	}
	if v <= 0 || v >= 100 {
		return 0, fmt.Errorf("target must be between 0 and 100 exclusive, got %f", v)
	}
	return v, nil
}

func ParseWindow(w v1alpha1.Window) time.Duration {
	switch w {
	case v1alpha1.Window7d:
		return 7 * 24 * time.Hour
	case v1alpha1.Window14d:
		return 14 * 24 * time.Hour
	case v1alpha1.Window28d:
		return 28 * 24 * time.Hour
	case v1alpha1.Window30d:
		return 30 * 24 * time.Hour
	case v1alpha1.Window90d:
		return 90 * 24 * time.Hour
	default:
		return 30 * 24 * time.Hour
	}
}

func BurnRateWindowsForSLO(window v1alpha1.Window) []BurnRateWindow {
	period := ParseWindow(window)
	base := DefaultBurnRateWindows()
	if period == 30*24*time.Hour {
		return base
	}
	return ScaleWindowsForPeriod(base, period)
}

func FormatDuration(d time.Duration) string {
	if d >= 24*time.Hour {
		days := int(d.Hours() / 24)
		return fmt.Sprintf("%dd", days)
	}
	if d >= time.Hour {
		hours := int(d.Hours())
		return fmt.Sprintf("%dh", hours)
	}
	minutes := int(d.Minutes())
	return fmt.Sprintf("%dm", minutes)
}
