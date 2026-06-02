package prometheus

import (
	"context"
	"strings"
	"testing"

	v1alpha1 "github.com/acabrera02/slo-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuildRulesYAML_Availability(t *testing.T) {
	b := &PrometheusBackend{}

	slo := &v1alpha1.ServiceLevelObjective{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-slo",
			Namespace: "default",
		},
		Spec: v1alpha1.ServiceLevelObjectiveSpec{
			Service: "api-gateway",
			SLI: v1alpha1.SLISpec{
				Type: v1alpha1.SLITypeAvailability,
				Availability: &v1alpha1.AvailabilitySLI{
					TotalQuery: `sum(rate(http_requests_total{service="api"}[{{.window}}]))`,
					ErrorQuery: `sum(rate(http_requests_total{service="api",code=~"5.."}[{{.window}}]))`,
				},
			},
			Target: "99.9",
			Window: v1alpha1.Window30d,
			Backends: []v1alpha1.BackendRef{
				{Name: v1alpha1.BackendPrometheus},
			},
		},
	}

	results, err := b.Reconcile(context.Background(), slo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// Call mutate to populate configmap data
	if err := results[0].MutateFunc(); err != nil {
		t.Fatalf("mutate error: %v", err)
	}
}

func TestBuildRulesYAML_ContainsBurnRateAlerts(t *testing.T) {
	b := &PrometheusBackend{}

	slo := &v1alpha1.ServiceLevelObjective{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-slo",
			Namespace: "default",
		},
		Spec: v1alpha1.ServiceLevelObjectiveSpec{
			Service: "myservice",
			SLI: v1alpha1.SLISpec{
				Type: v1alpha1.SLITypeAvailability,
				Availability: &v1alpha1.AvailabilitySLI{
					TotalQuery: `sum(rate(http_total[{{.window}}]))`,
					ErrorQuery: `sum(rate(http_errors[{{.window}}]))`,
				},
			},
			Target: "99.9",
			Window: v1alpha1.Window30d,
			Backends: []v1alpha1.BackendRef{
				{Name: v1alpha1.BackendPrometheus},
			},
		},
	}

	yaml, err := b.buildRulesYAML(slo, 99.9)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	checks := []string{
		"SLOBurnRateHigh",
		"severity: critical",
		"severity: warning",
		"slo:error_ratio:rate",
		"slo_name: myservice-availability",
		"service: myservice",
	}

	for _, check := range checks {
		if !strings.Contains(yaml, check) {
			t.Errorf("generated YAML missing %q", check)
		}
	}
}

func TestBuildRulesYAML_AlertsDisabled(t *testing.T) {
	b := &PrometheusBackend{}

	slo := &v1alpha1.ServiceLevelObjective{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-slo",
			Namespace: "default",
		},
		Spec: v1alpha1.ServiceLevelObjectiveSpec{
			Service: "myservice",
			SLI: v1alpha1.SLISpec{
				Type: v1alpha1.SLITypeAvailability,
				Availability: &v1alpha1.AvailabilitySLI{
					TotalQuery: `sum(rate(http_total[{{.window}}]))`,
					ErrorQuery: `sum(rate(http_errors[{{.window}}]))`,
				},
			},
			Target: "99.9",
			Window: v1alpha1.Window30d,
			Alerting: &v1alpha1.AlertingSpec{
				Disabled: true,
			},
			Backends: []v1alpha1.BackendRef{
				{Name: v1alpha1.BackendPrometheus},
			},
		},
	}

	yaml, err := b.buildRulesYAML(slo, 99.9)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(yaml, "SLOBurnRateHigh") {
		t.Error("expected no alerts when alerting is disabled")
	}

	if !strings.Contains(yaml, "slo:error_ratio:rate") {
		t.Error("recording rules should still be generated even when alerts disabled")
	}
}
