package datadog

import (
	"context"
	"encoding/json"
	"testing"

	v1alpha1 "github.com/AndreCbrera/slo-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestReconcile_CreatesMonitors(t *testing.T) {
	d := &DatadogBackend{}

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
					TotalQuery: `sum:http.requests.total{service:api-gateway}`,
					ErrorQuery: `sum:http.requests.total{service:api-gateway,code:5xx}`,
				},
			},
			Target: "99.9",
			Window: v1alpha1.Window30d,
			Backends: []v1alpha1.BackendRef{
				{Name: v1alpha1.BackendDatadog},
			},
		},
	}

	results, err := d.Reconcile(context.Background(), slo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].Object.GetName() != "slo-test-slo-datadog" {
		t.Errorf("unexpected name: %s", results[0].Object.GetName())
	}
}

func TestBuildMonitors_FourBurnRates(t *testing.T) {
	d := &DatadogBackend{}

	slo := &v1alpha1.ServiceLevelObjective{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: v1alpha1.ServiceLevelObjectiveSpec{
			Service: "api",
			SLI: v1alpha1.SLISpec{
				Type: v1alpha1.SLITypeAvailability,
				Availability: &v1alpha1.AvailabilitySLI{
					TotalQuery: "total",
					ErrorQuery: "errors",
				},
			},
			Target: "99.9",
			Window: v1alpha1.Window30d,
		},
	}

	monitors := d.buildMonitors(slo, 99.9)
	if len(monitors) != 4 {
		t.Fatalf("expected 4 monitors, got %d", len(monitors))
	}

	// Verify JSON serialization works
	_, err := json.Marshal(monitors)
	if err != nil {
		t.Fatalf("failed to marshal monitors: %v", err)
	}

	// Check tags
	for _, m := range monitors {
		hasSLOTag := false
		for _, tag := range m.Tags {
			if tag == "slo:api-availability" {
				hasSLOTag = true
			}
		}
		if !hasSLOTag {
			t.Errorf("monitor %q missing slo tag", m.Name)
		}
	}
}
