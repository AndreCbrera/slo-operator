package prometheus

import (
	"context"
	"testing"

	v1alpha1 "github.com/AndreCbrera/slo-operator/api/v1alpha1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestReconcile_Availability(t *testing.T) {
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

	rule, ok := results[0].Object.(*monitoringv1.PrometheusRule)
	if !ok {
		t.Fatal("expected PrometheusRule object")
	}

	if rule.Name != "slo-test-slo" {
		t.Errorf("unexpected name: %s", rule.Name)
	}
}

func TestBuildSpec_ContainsBurnRateAlerts(t *testing.T) {
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

	spec := b.buildSpec(slo, 99.9)

	if len(spec.Groups) != 2 {
		t.Fatalf("expected 2 rule groups (recording + alerts), got %d", len(spec.Groups))
	}

	recordingGroup := spec.Groups[0]
	if recordingGroup.Name != "slo-myservice-availability-recording" {
		t.Errorf("unexpected recording group name: %s", recordingGroup.Name)
	}
	if len(recordingGroup.Rules) == 0 {
		t.Error("expected recording rules")
	}

	alertGroup := spec.Groups[1]
	if alertGroup.Name != "slo-myservice-availability-alerts" {
		t.Errorf("unexpected alert group name: %s", alertGroup.Name)
	}
	if len(alertGroup.Rules) != 4 {
		t.Errorf("expected 4 alert rules (multi-burn-rate), got %d", len(alertGroup.Rules))
	}

	// Check severities
	critCount := 0
	warnCount := 0
	for _, rule := range alertGroup.Rules {
		if rule.Alert != "SLOBurnRateHigh" {
			t.Errorf("unexpected alert name: %s", rule.Alert)
		}
		switch rule.Labels["severity"] {
		case "critical":
			critCount++
		case "warning":
			warnCount++
		}
	}
	if critCount != 2 {
		t.Errorf("expected 2 critical alerts, got %d", critCount)
	}
	if warnCount != 2 {
		t.Errorf("expected 2 warning alerts, got %d", warnCount)
	}
}

func TestBuildSpec_AlertsDisabled(t *testing.T) {
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

	spec := b.buildSpec(slo, 99.9)

	if len(spec.Groups) != 1 {
		t.Fatalf("expected 1 rule group (recording only), got %d", len(spec.Groups))
	}

	if spec.Groups[0].Name != "slo-myservice-availability-recording" {
		t.Errorf("unexpected group name: %s", spec.Groups[0].Name)
	}
}
