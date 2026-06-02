package grafana

import (
	"context"
	"encoding/json"
	"testing"

	v1alpha1 "github.com/AndreCbrera/slo-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuildDashboardJSON(t *testing.T) {
	g := &GrafanaBackend{}

	slo := &v1alpha1.ServiceLevelObjective{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-slo",
			Namespace: "default",
		},
		Spec: v1alpha1.ServiceLevelObjectiveSpec{
			Service: "api-gateway",
			SLI: v1alpha1.SLISpec{
				Type: v1alpha1.SLITypeAvailability,
			},
			Target: "99.9",
			Window: v1alpha1.Window30d,
			Backends: []v1alpha1.BackendRef{
				{Name: v1alpha1.BackendGrafana},
			},
		},
	}

	dashJSON, err := g.buildDashboardJSON(slo, 99.9)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var dash map[string]interface{}
	if err := json.Unmarshal([]byte(dashJSON), &dash); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if dash["title"] != "SLO: api-gateway-availability" {
		t.Errorf("unexpected title: %v", dash["title"])
	}

	panels, ok := dash["panels"].([]interface{})
	if !ok {
		t.Fatal("panels is not an array")
	}
	if len(panels) != 4 {
		t.Errorf("expected 4 panels, got %d", len(panels))
	}
}

func TestReconcile_CreatesConfigMap(t *testing.T) {
	g := &GrafanaBackend{}

	slo := &v1alpha1.ServiceLevelObjective{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-slo",
			Namespace: "monitoring",
		},
		Spec: v1alpha1.ServiceLevelObjectiveSpec{
			Service: "checkout",
			SLI: v1alpha1.SLISpec{
				Type: v1alpha1.SLITypeLatency,
				Latency: &v1alpha1.LatencySLI{
					TotalQuery:   `sum(rate(http_duration_count[{{.window}}]))`,
					SuccessQuery: `sum(rate(http_duration_bucket{le="0.5"}[{{.window}}]))`,
					ThresholdMs:  500,
				},
			},
			Target: "99.0",
			Window: v1alpha1.Window7d,
			Backends: []v1alpha1.BackendRef{
				{Name: v1alpha1.BackendGrafana, Config: map[string]string{"folder": "MyFolder"}},
			},
		},
	}

	results, err := g.Reconcile(context.Background(), slo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	obj := results[0].Object
	if obj.GetName() != "slo-my-slo-dashboard" {
		t.Errorf("unexpected configmap name: %s", obj.GetName())
	}
	if obj.GetNamespace() != "monitoring" {
		t.Errorf("unexpected namespace: %s", obj.GetNamespace())
	}

	annotations := obj.GetAnnotations()
	if annotations["grafana-folder"] != "MyFolder" {
		t.Errorf("unexpected folder annotation: %s", annotations["grafana-folder"])
	}
}
