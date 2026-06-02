package prometheus

import (
	"context"
	"fmt"
	"strings"

	v1alpha1 "github.com/acabrera02/slo-operator/api/v1alpha1"
	"github.com/acabrera02/slo-operator/internal/backend"
	"github.com/acabrera02/slo-operator/internal/slo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PrometheusBackend struct {
	client client.Client
}

func New(c client.Client) *PrometheusBackend {
	return &PrometheusBackend{client: c}
}

func (p *PrometheusBackend) Name() string {
	return "prometheus"
}

func (p *PrometheusBackend) Reconcile(ctx context.Context, obj *v1alpha1.ServiceLevelObjective) ([]backend.Result, error) {
	target, err := slo.ParseTarget(obj.Spec.Target)
	if err != nil {
		return nil, err
	}

	rulesYAML, err := p.buildRulesYAML(obj, target)
	if err != nil {
		return nil, fmt.Errorf("building prometheus rules: %w", err)
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("slo-%s-rules", obj.Name),
			Namespace: obj.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "slo-operator",
				"prometheus":                   "rules",
				"slo.sre.io/name":              obj.Name,
			},
		},
	}

	mutate := func() error {
		cm.Data = map[string]string{
			fmt.Sprintf("slo-%s.yaml", obj.Name): rulesYAML,
		}
		return nil
	}

	return []backend.Result{{Object: cm, MutateFunc: mutate}}, nil
}

func (p *PrometheusBackend) Cleanup(ctx context.Context, obj *v1alpha1.ServiceLevelObjective) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("slo-%s-rules", obj.Name),
			Namespace: obj.Namespace,
		},
	}
	return client.IgnoreNotFound(p.client.Delete(ctx, cm))
}

func (p *PrometheusBackend) buildRulesYAML(obj *v1alpha1.ServiceLevelObjective, target float64) (string, error) {
	maxError := slo.MaxErrorRate(target)
	windows := slo.BurnRateWindowsForSLO(obj.Spec.Window)

	sloName := fmt.Sprintf("%s-%s", obj.Spec.Service, string(obj.Spec.SLI.Type))
	errorExpr, totalExpr := p.getQueryExprs(obj)

	var sb strings.Builder
	sb.WriteString("groups:\n")

	// Recording rules group
	sb.WriteString(fmt.Sprintf("  - name: slo-%s-recording\n", sloName))
	sb.WriteString("    rules:\n")

	uniqueWindows := collectUniqueWindowDurations(windows)
	for _, wStr := range uniqueWindows {
		sb.WriteString(fmt.Sprintf("      - record: slo:error_ratio:rate%s\n", wStr))
		sb.WriteString(fmt.Sprintf("        expr: |\n          (%s) / (%s)\n", substituteWindow(errorExpr, wStr), substituteWindow(totalExpr, wStr)))
		sb.WriteString(fmt.Sprintf("        labels:\n          slo_name: %s\n", sloName))
	}

	// Alert rules group
	if obj.Spec.Alerting != nil && obj.Spec.Alerting.Disabled {
		return sb.String(), nil
	}

	sb.WriteString(fmt.Sprintf("  - name: slo-%s-alerts\n", sloName))
	sb.WriteString("    rules:\n")

	for _, bw := range windows {
		longStr := slo.FormatDuration(bw.LongWindow)
		shortStr := slo.FormatDuration(bw.ShortWindow)
		threshold := fmt.Sprintf("%.6f", bw.BurnRate*maxError)
		forStr := slo.FormatDuration(bw.For)

		sb.WriteString("      - alert: SLOBurnRateHigh\n")
		sb.WriteString(fmt.Sprintf("        expr: |\n          slo:error_ratio:rate%s{slo_name=\"%s\"} > %s\n          and\n          slo:error_ratio:rate%s{slo_name=\"%s\"} > %s\n",
			longStr, sloName, threshold,
			shortStr, sloName, threshold))
		sb.WriteString(fmt.Sprintf("        for: %s\n", forStr))
		sb.WriteString("        labels:\n")
		sb.WriteString(fmt.Sprintf("          severity: %s\n", bw.Severity))
		sb.WriteString(fmt.Sprintf("          slo_name: %s\n", sloName))
		sb.WriteString(fmt.Sprintf("          service: %s\n", obj.Spec.Service))

		if obj.Spec.Alerting != nil {
			for k, v := range obj.Spec.Alerting.Labels {
				sb.WriteString(fmt.Sprintf("          %s: %s\n", k, v))
			}
		}

		sb.WriteString("        annotations:\n")
		sb.WriteString(fmt.Sprintf("          summary: \"High burn rate on SLO %s\"\n", sloName))
		sb.WriteString(fmt.Sprintf("          description: \"Error budget consumed %.1fx faster than allowed (window: %s/%s)\"\n", bw.BurnRate, longStr, shortStr))

		if obj.Spec.Alerting != nil {
			for k, v := range obj.Spec.Alerting.Annotations {
				sb.WriteString(fmt.Sprintf("          %s: %s\n", k, v))
			}
		}
	}

	return sb.String(), nil
}

func (p *PrometheusBackend) getQueryExprs(obj *v1alpha1.ServiceLevelObjective) (errorExpr, totalExpr string) {
	switch {
	case obj.Spec.SLI.Availability != nil:
		return obj.Spec.SLI.Availability.ErrorQuery, obj.Spec.SLI.Availability.TotalQuery
	case obj.Spec.SLI.Latency != nil:
		totalExpr := obj.Spec.SLI.Latency.TotalQuery
		goodExpr := obj.Spec.SLI.Latency.SuccessQuery
		return fmt.Sprintf("(%s) - (%s)", totalExpr, goodExpr), totalExpr
	case obj.Spec.SLI.Raw != nil:
		totalExpr := obj.Spec.SLI.Raw.TotalEventsQuery
		goodExpr := obj.Spec.SLI.Raw.GoodEventsQuery
		return fmt.Sprintf("(%s) - (%s)", totalExpr, goodExpr), totalExpr
	default:
		return "unknown", "unknown"
	}
}

func substituteWindow(query, window string) string {
	return strings.ReplaceAll(query, "{{.window}}", window)
}

func collectUniqueWindowDurations(bws []slo.BurnRateWindow) []string {
	seen := make(map[string]bool)
	var result []string
	for _, bw := range bws {
		for _, d := range []string{slo.FormatDuration(bw.LongWindow), slo.FormatDuration(bw.ShortWindow)} {
			if !seen[d] {
				seen[d] = true
				result = append(result, d)
			}
		}
	}
	return result
}
