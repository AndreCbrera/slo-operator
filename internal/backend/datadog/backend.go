package datadog

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

type DatadogBackend struct {
	client client.Client
}

func New(c client.Client) *DatadogBackend {
	return &DatadogBackend{client: c}
}

func (d *DatadogBackend) Name() string {
	return "datadog"
}

func (d *DatadogBackend) Reconcile(ctx context.Context, obj *v1alpha1.ServiceLevelObjective) ([]backend.Result, error) {
	target, err := slo.ParseTarget(obj.Spec.Target)
	if err != nil {
		return nil, err
	}

	monitors := d.buildMonitors(obj, target)
	monitorsJSON, err := json.MarshalIndent(monitors, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling datadog monitors: %w", err)
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("slo-%s-datadog", obj.Name),
			Namespace: obj.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "slo-operator",
				"slo.sre.io/name":              obj.Name,
				"slo.sre.io/backend":           "datadog",
			},
		},
	}

	mutate := func() error {
		cm.Data = map[string]string{
			"monitors.json": string(monitorsJSON),
		}
		return nil
	}

	return []backend.Result{{Object: cm, MutateFunc: mutate}}, nil
}

func (d *DatadogBackend) Cleanup(ctx context.Context, obj *v1alpha1.ServiceLevelObjective) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("slo-%s-datadog", obj.Name),
			Namespace: obj.Namespace,
		},
	}
	return client.IgnoreNotFound(d.client.Delete(ctx, cm))
}

type DatadogMonitor struct {
	Name    string            `json:"name"`
	Type    string            `json:"type"`
	Query   string            `json:"query"`
	Message string            `json:"message"`
	Tags    []string          `json:"tags"`
	Options MonitorOptions    `json:"options"`
}

type MonitorOptions struct {
	Thresholds MonitorThresholds `json:"thresholds"`
	NotifyNoData bool           `json:"notify_no_data"`
	RenotifyInterval int        `json:"renotify_interval"`
}

type MonitorThresholds struct {
	Critical string `json:"critical"`
	Warning  string `json:"warning,omitempty"`
}

func (d *DatadogBackend) buildMonitors(obj *v1alpha1.ServiceLevelObjective, target float64) []DatadogMonitor {
	sloName := fmt.Sprintf("%s-%s", obj.Spec.Service, string(obj.Spec.SLI.Type))
	maxError := slo.MaxErrorRate(target)
	windows := slo.BurnRateWindowsForSLO(obj.Spec.Window)

	var monitors []DatadogMonitor

	for _, bw := range windows {
		longStr := slo.FormatDuration(bw.LongWindow)
		threshold := bw.BurnRate * maxError

		query := fmt.Sprintf(
			"avg(last_%s):sum:slo.error_ratio{slo_name:%s} > %f",
			longStr, sloName, threshold,
		)

		monitor := DatadogMonitor{
			Name:    fmt.Sprintf("[SLO] %s - Burn Rate %.1fx (%s)", sloName, bw.BurnRate, longStr),
			Type:    "query alert",
			Query:   query,
			Message: fmt.Sprintf("SLO %s is burning error budget %.1fx faster than allowed.\n\nService: %s\nTarget: %.2f%%\n\n@slack-slo-alerts", sloName, bw.BurnRate, obj.Spec.Service, target),
			Tags: []string{
				fmt.Sprintf("service:%s", obj.Spec.Service),
				fmt.Sprintf("slo:%s", sloName),
				fmt.Sprintf("severity:%s", bw.Severity),
				"managed-by:slo-operator",
			},
			Options: MonitorOptions{
				Thresholds: MonitorThresholds{
					Critical: fmt.Sprintf("%f", threshold),
				},
				NotifyNoData:     false,
				RenotifyInterval: 240,
			},
		}
		monitors = append(monitors, monitor)
	}

	return monitors
}
