package grafana

import (
	"context"
	"encoding/json"
	"fmt"

	v1alpha1 "github.com/AndreCbrera/slo-operator/api/v1alpha1"
	"github.com/AndreCbrera/slo-operator/internal/backend"
	"github.com/AndreCbrera/slo-operator/internal/slo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type GrafanaBackend struct {
	client client.Client
}

func New(c client.Client) *GrafanaBackend {
	return &GrafanaBackend{client: c}
}

func (g *GrafanaBackend) Name() string {
	return "grafana"
}

func (g *GrafanaBackend) Reconcile(ctx context.Context, obj *v1alpha1.ServiceLevelObjective) ([]backend.Result, error) {
	target, err := slo.ParseTarget(obj.Spec.Target)
	if err != nil {
		return nil, err
	}

	dashJSON, err := g.buildDashboardJSON(obj, target)
	if err != nil {
		return nil, fmt.Errorf("building grafana dashboard: %w", err)
	}

	folder := "SLOs"
	if obj.Spec.Backends != nil {
		for _, b := range obj.Spec.Backends {
			if b.Name == v1alpha1.BackendGrafana && b.Config != nil {
				if f, ok := b.Config["folder"]; ok {
					folder = f
				}
			}
		}
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("slo-%s-dashboard", obj.Name),
			Namespace: obj.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "slo-operator",
				"grafana_dashboard":            "1",
				"slo.sre.io/name":              obj.Name,
			},
			Annotations: map[string]string{
				"grafana-folder": folder,
			},
		},
	}

	mutate := func() error {
		cm.Data = map[string]string{
			fmt.Sprintf("slo-%s.json", obj.Name): dashJSON,
		}
		return nil
	}

	return []backend.Result{{Object: cm, MutateFunc: mutate}}, nil
}

func (g *GrafanaBackend) Cleanup(ctx context.Context, obj *v1alpha1.ServiceLevelObjective) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("slo-%s-dashboard", obj.Name),
			Namespace: obj.Namespace,
		},
	}
	return client.IgnoreNotFound(g.client.Delete(ctx, cm))
}

func (g *GrafanaBackend) buildDashboardJSON(obj *v1alpha1.ServiceLevelObjective, target float64) (string, error) {
	sloName := fmt.Sprintf("%s-%s", obj.Spec.Service, string(obj.Spec.SLI.Type))
	maxError := slo.MaxErrorRate(target)

	dashboard := map[string]interface{}{
		"title":         fmt.Sprintf("SLO: %s", sloName),
		"uid":           fmt.Sprintf("slo-%s", obj.Name),
		"schemaVersion": 39,
		"editable":      false,
		"tags":          []string{"slo", "generated", obj.Spec.Service},
		"annotations": map[string]interface{}{
			"list": []interface{}{},
		},
		"panels": g.buildPanels(sloName, target, maxError),
		"time": map[string]string{
			"from": "now-" + string(obj.Spec.Window),
			"to":   "now",
		},
		"refresh": "1m",
	}

	data, err := json.MarshalIndent(dashboard, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (g *GrafanaBackend) buildPanels(sloName string, target, maxError float64) []map[string]interface{} {
	return []map[string]interface{}{
		{
			"title":    "SLI - Current Error Ratio",
			"type":     "gauge",
			"id":       1,
			"gridPos":  map[string]int{"h": 8, "w": 6, "x": 0, "y": 0},
			"fieldConfig": map[string]interface{}{
				"defaults": map[string]interface{}{
					"thresholds": map[string]interface{}{
						"steps": []map[string]interface{}{
							{"color": "green", "value": nil},
							{"color": "yellow", "value": maxError * 0.5},
							{"color": "red", "value": maxError},
						},
					},
					"max":  maxError * 2,
					"unit": "percentunit",
				},
			},
			"targets": []map[string]interface{}{
				{
					"expr":         fmt.Sprintf(`slo:error_ratio:rate1h{slo_name="%s"}`, sloName),
					"legendFormat": "Error Ratio (1h)",
				},
			},
		},
		{
			"title":   "Error Budget Remaining",
			"type":    "timeseries",
			"id":      2,
			"gridPos": map[string]int{"h": 8, "w": 12, "x": 6, "y": 0},
			"fieldConfig": map[string]interface{}{
				"defaults": map[string]interface{}{
					"unit": "percentunit",
					"thresholds": map[string]interface{}{
						"steps": []map[string]interface{}{
							{"color": "red", "value": nil},
							{"color": "yellow", "value": 0.25},
							{"color": "green", "value": 0.5},
						},
					},
				},
			},
			"targets": []map[string]interface{}{
				{
					"expr":         fmt.Sprintf(`1 - (slo:error_ratio:rate1h{slo_name="%s"} / %f)`, sloName, maxError),
					"legendFormat": "Budget Remaining",
				},
			},
		},
		{
			"title":   "Burn Rate",
			"type":    "timeseries",
			"id":      3,
			"gridPos": map[string]int{"h": 8, "w": 6, "x": 18, "y": 0},
			"fieldConfig": map[string]interface{}{
				"defaults": map[string]interface{}{
					"thresholds": map[string]interface{}{
						"steps": []map[string]interface{}{
							{"color": "green", "value": nil},
							{"color": "yellow", "value": 1},
							{"color": "orange", "value": 3},
							{"color": "red", "value": 6},
						},
					},
				},
			},
			"targets": []map[string]interface{}{
				{
					"expr":         fmt.Sprintf(`slo:error_ratio:rate1h{slo_name="%s"} / %f`, sloName, maxError),
					"legendFormat": "Burn Rate (1h)",
				},
				{
					"expr":         fmt.Sprintf(`slo:error_ratio:rate6h{slo_name="%s"} / %f`, sloName, maxError),
					"legendFormat": "Burn Rate (6h)",
				},
			},
		},
		{
			"title":   "SLI Over Time",
			"type":    "timeseries",
			"id":      4,
			"gridPos": map[string]int{"h": 8, "w": 24, "x": 0, "y": 8},
			"fieldConfig": map[string]interface{}{
				"defaults": map[string]interface{}{
					"unit": "percentunit",
					"custom": map[string]interface{}{
						"fillOpacity": 10,
					},
				},
			},
			"targets": []map[string]interface{}{
				{
					"expr":         fmt.Sprintf(`1 - slo:error_ratio:rate5m{slo_name="%s"}`, sloName),
					"legendFormat": "SLI (5m)",
				},
				{
					"expr":         fmt.Sprintf(`%f`, target/100.0),
					"legendFormat": fmt.Sprintf("Target (%.2f%%)", target),
				},
			},
		},
	}
}
