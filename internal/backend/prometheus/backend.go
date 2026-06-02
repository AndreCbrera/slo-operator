package prometheus

import (
	"context"
	"fmt"
	"strings"

	v1alpha1 "github.com/AndreCbrera/slo-operator/api/v1alpha1"
	"github.com/AndreCbrera/slo-operator/internal/backend"
	"github.com/AndreCbrera/slo-operator/internal/slo"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
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

	rule := p.buildPrometheusRule(obj, target)

	mutate := func() error {
		rule.Spec = p.buildSpec(obj, target)
		return nil
	}

	return []backend.Result{{Object: rule, MutateFunc: mutate}}, nil
}

func (p *PrometheusBackend) Cleanup(ctx context.Context, obj *v1alpha1.ServiceLevelObjective) error {
	rule := &monitoringv1.PrometheusRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("slo-%s", obj.Name),
			Namespace: obj.Namespace,
		},
	}
	return client.IgnoreNotFound(p.client.Delete(ctx, rule))
}

func (p *PrometheusBackend) buildPrometheusRule(obj *v1alpha1.ServiceLevelObjective, target float64) *monitoringv1.PrometheusRule {
	return &monitoringv1.PrometheusRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("slo-%s", obj.Name),
			Namespace: obj.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "slo-operator",
				"slo.sre.io/name":              obj.Name,
			},
		},
		Spec: p.buildSpec(obj, target),
	}
}

func (p *PrometheusBackend) buildSpec(obj *v1alpha1.ServiceLevelObjective, target float64) monitoringv1.PrometheusRuleSpec {
	maxError := slo.MaxErrorRate(target)
	windows := slo.BurnRateWindowsForSLO(obj.Spec.Window)
	sloName := fmt.Sprintf("%s-%s", obj.Spec.Service, string(obj.Spec.SLI.Type))
	errorExpr, totalExpr := p.getQueryExprs(obj)

	var groups []monitoringv1.RuleGroup

	// Recording rules
	var recordingRules []monitoringv1.Rule
	uniqueWindows := collectUniqueWindowDurations(windows)
	for _, wStr := range uniqueWindows {
		recordingRules = append(recordingRules, monitoringv1.Rule{
			Record: fmt.Sprintf("slo:error_ratio:rate%s", wStr),
			Expr:   intstr.FromString(fmt.Sprintf("(%s) / (%s)", substituteWindow(errorExpr, wStr), substituteWindow(totalExpr, wStr))),
			Labels: map[string]string{"slo_name": sloName},
		})
	}
	groups = append(groups, monitoringv1.RuleGroup{
		Name:  fmt.Sprintf("slo-%s-recording", sloName),
		Rules: recordingRules,
	})

	// Alert rules
	if obj.Spec.Alerting == nil || !obj.Spec.Alerting.Disabled {
		var alertRules []monitoringv1.Rule
		for _, bw := range windows {
			longStr := slo.FormatDuration(bw.LongWindow)
			shortStr := slo.FormatDuration(bw.ShortWindow)
			threshold := fmt.Sprintf("%.6f", bw.BurnRate*maxError)
			forDuration := monitoringv1.Duration(slo.FormatDuration(bw.For))

			expr := fmt.Sprintf(
				`slo:error_ratio:rate%s{slo_name="%s"} > %s and slo:error_ratio:rate%s{slo_name="%s"} > %s`,
				longStr, sloName, threshold,
				shortStr, sloName, threshold,
			)

			labels := map[string]string{
				"severity": bw.Severity,
				"slo_name": sloName,
				"service":  obj.Spec.Service,
			}
			if obj.Spec.Alerting != nil {
				for k, v := range obj.Spec.Alerting.Labels {
					labels[k] = v
				}
			}

			annotations := map[string]string{
				"summary":     fmt.Sprintf("High burn rate on SLO %s", sloName),
				"description": fmt.Sprintf("Error budget consumed %.1fx faster than allowed (window: %s/%s)", bw.BurnRate, longStr, shortStr),
			}
			if obj.Spec.Alerting != nil {
				for k, v := range obj.Spec.Alerting.Annotations {
					annotations[k] = v
				}
			}

			alertRules = append(alertRules, monitoringv1.Rule{
				Alert:       "SLOBurnRateHigh",
				Expr:        intstr.FromString(expr),
				For:         &forDuration,
				Labels:      labels,
				Annotations: annotations,
			})
		}
		groups = append(groups, monitoringv1.RuleGroup{
			Name:  fmt.Sprintf("slo-%s-alerts", sloName),
			Rules: alertRules,
		})
	}

	return monitoringv1.PrometheusRuleSpec{Groups: groups}
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
