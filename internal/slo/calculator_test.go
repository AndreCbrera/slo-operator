package slo

import (
	"testing"
	"time"

	v1alpha1 "github.com/AndreCbrera/slo-operator/api/v1alpha1"
)

func TestMaxErrorRate(t *testing.T) {
	tests := []struct {
		name   string
		target float64
		want   float64
	}{
		{"99.9% SLO", 99.9, 0.001},
		{"99% SLO", 99.0, 0.01},
		{"99.99% SLO", 99.99, 0.0001},
		{"95% SLO", 95.0, 0.05},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MaxErrorRate(tt.target)
			if diff := got - tt.want; diff > 1e-10 || diff < -1e-10 {
				t.Errorf("MaxErrorRate(%f) = %f, want %f", tt.target, got, tt.want)
			}
		})
	}
}

func TestBurnRate(t *testing.T) {
	tests := []struct {
		name            string
		actualErrorRate float64
		maxErrorRate    float64
		want            float64
	}{
		{"exactly at budget", 0.001, 0.001, 1.0},
		{"2x burn", 0.002, 0.001, 2.0},
		{"14.4x burn", 0.0144, 0.001, 14.4},
		{"no errors", 0.0, 0.001, 0.0},
		{"zero max", 0.001, 0.0, 0.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BurnRate(tt.actualErrorRate, tt.maxErrorRate)
			if diff := got - tt.want; diff > 1e-10 || diff < -1e-10 {
				t.Errorf("BurnRate(%f, %f) = %f, want %f", tt.actualErrorRate, tt.maxErrorRate, got, tt.want)
			}
		})
	}
}

func TestParseTarget(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    float64
		wantErr bool
	}{
		{"valid 99.9", "99.9", 99.9, false},
		{"valid 99", "99", 99.0, false},
		{"valid 99.99", "99.99", 99.99, false},
		{"invalid zero", "0", 0, true},
		{"invalid 100", "100", 0, true},
		{"invalid text", "abc", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTarget(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseTarget(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseTarget(%q) = %f, want %f", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseWindow(t *testing.T) {
	tests := []struct {
		input v1alpha1.Window
		want  time.Duration
	}{
		{v1alpha1.Window7d, 7 * 24 * time.Hour},
		{v1alpha1.Window14d, 14 * 24 * time.Hour},
		{v1alpha1.Window28d, 28 * 24 * time.Hour},
		{v1alpha1.Window30d, 30 * 24 * time.Hour},
		{v1alpha1.Window90d, 90 * 24 * time.Hour},
	}
	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			got := ParseWindow(tt.input)
			if got != tt.want {
				t.Errorf("ParseWindow(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		input time.Duration
		want  string
	}{
		{5 * time.Minute, "5m"},
		{30 * time.Minute, "30m"},
		{1 * time.Hour, "1h"},
		{6 * time.Hour, "6h"},
		{24 * time.Hour, "1d"},
		{3 * 24 * time.Hour, "3d"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := FormatDuration(tt.input)
			if got != tt.want {
				t.Errorf("FormatDuration(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBurnRateWindowsForSLO_30d(t *testing.T) {
	windows := BurnRateWindowsForSLO(v1alpha1.Window30d)
	if len(windows) != 4 {
		t.Fatalf("expected 4 windows, got %d", len(windows))
	}

	expected := []struct {
		severity string
		burnRate float64
	}{
		{"critical", 14.4},
		{"critical", 6.0},
		{"warning", 3.0},
		{"warning", 1.0},
	}

	for i, e := range expected {
		if windows[i].Severity != e.severity {
			t.Errorf("window[%d].Severity = %q, want %q", i, windows[i].Severity, e.severity)
		}
		if windows[i].BurnRate != e.burnRate {
			t.Errorf("window[%d].BurnRate = %f, want %f", i, windows[i].BurnRate, e.burnRate)
		}
	}
}

func TestScaleWindowsForPeriod_7d(t *testing.T) {
	base := DefaultBurnRateWindows()
	sevenDays := 7 * 24 * time.Hour
	scaled := ScaleWindowsForPeriod(base, sevenDays)

	if len(scaled) != 4 {
		t.Fatalf("expected 4 windows, got %d", len(scaled))
	}

	// 7d is ~23% of 30d, so 1h long window scales to ~14m, but minimum is 5m
	for _, w := range scaled {
		if w.LongWindow < 5*time.Minute {
			t.Errorf("LongWindow %v is below 5m minimum", w.LongWindow)
		}
		if w.ShortWindow < 1*time.Minute {
			t.Errorf("ShortWindow %v is below 1m minimum", w.ShortWindow)
		}
	}
}
