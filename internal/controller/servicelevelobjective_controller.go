/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"

	slov1alpha1 "github.com/AndreCbrera/slo-operator/api/v1alpha1"
	"github.com/AndreCbrera/slo-operator/internal/backend"
)

var (
	reconcileDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "slo_operator_reconcile_duration_seconds",
			Help:    "Duration of SLO reconciliation",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"name", "namespace"},
	)

	reconcileErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "slo_operator_reconcile_errors_total",
			Help: "Total number of reconciliation errors",
		},
		[]string{"name", "namespace", "backend"},
	)

	sloCount = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "slo_operator_managed_slos",
			Help: "Number of SLOs currently managed",
		},
	)
)

func init() {
	metrics.Registry.MustRegister(reconcileDuration, reconcileErrors, sloCount)
}

const finalizerName = "slo.sre.io/cleanup"

type ServiceLevelObjectiveReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Registry *backend.Registry
}

// +kubebuilder:rbac:groups=slo.sre.io,resources=servicelevelobjectives,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=slo.sre.io,resources=servicelevelobjectives/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=slo.sre.io,resources=servicelevelobjectives/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=prometheusrules,verbs=get;list;watch;create;update;patch;delete

func (r *ServiceLevelObjectiveReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	start := time.Now()
	defer func() {
		reconcileDuration.WithLabelValues(req.Name, req.Namespace).Observe(time.Since(start).Seconds())
	}()

	var slo slov1alpha1.ServiceLevelObjective
	if err := r.Get(ctx, req.NamespacedName, &slo); err != nil {
		if errors.IsNotFound(err) {
			sloCount.Dec()
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !slo.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&slo, finalizerName) {
			if err := r.cleanup(ctx, &slo); err != nil {
				log.Error(err, "failed to cleanup resources")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(&slo, finalizerName)
			if err := r.Update(ctx, &slo); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(&slo, finalizerName) {
		controllerutil.AddFinalizer(&slo, finalizerName)
		if err := r.Update(ctx, &slo); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Reconcile backends
	var generatedResources []slov1alpha1.GeneratedResource
	for _, backendRef := range slo.Spec.Backends {
		b, err := r.Registry.Get(backendRef.Name)
		if err != nil {
			log.Error(err, "backend not found", "backend", backendRef.Name)
			r.setCondition(&slo, "Ready", metav1.ConditionFalse, "BackendNotFound", err.Error())
			if statusErr := r.Status().Update(ctx, &slo); statusErr != nil {
				log.Error(statusErr, "failed to update status")
			}
			continue
		}

		results, err := b.Reconcile(ctx, &slo)
		if err != nil {
			reconcileErrors.WithLabelValues(slo.Name, slo.Namespace, b.Name()).Inc()
			log.Error(err, "backend reconcile failed", "backend", backendRef.Name)
			r.setCondition(&slo, "Ready", metav1.ConditionFalse, "ReconcileFailed", err.Error())
			if statusErr := r.Status().Update(ctx, &slo); statusErr != nil {
				log.Error(statusErr, "failed to update status")
			}
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}

		for _, result := range results {
			if err := controllerutil.SetControllerReference(&slo, result.Object, r.Scheme); err != nil {
				return ctrl.Result{}, fmt.Errorf("setting owner reference: %w", err)
			}

			op, err := controllerutil.CreateOrUpdate(ctx, r.Client, result.Object, result.MutateFunc)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("create/update resource: %w", err)
			}
			log.Info("reconciled resource", "operation", op, "name", result.Object.GetName())

			generatedResources = append(generatedResources, slov1alpha1.GeneratedResource{
				Kind:      result.Object.GetObjectKind().GroupVersionKind().Kind,
				Name:      result.Object.GetName(),
				Namespace: result.Object.GetNamespace(),
				Backend:   b.Name(),
			})
		}
	}

	// Prune orphans
	r.pruneOrphans(ctx, &slo, generatedResources)

	// Update status
	if slo.Status.ObservedGeneration == 0 {
		sloCount.Inc()
	}
	now := metav1.Now()
	slo.Status.ObservedGeneration = slo.Generation
	slo.Status.GeneratedResources = generatedResources
	slo.Status.LastReconcileTime = &now
	r.setCondition(&slo, "Ready", metav1.ConditionTrue, "Reconciled", "All backends reconciled successfully")

	if err := r.Status().Update(ctx, &slo); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *ServiceLevelObjectiveReconciler) cleanup(ctx context.Context, slo *slov1alpha1.ServiceLevelObjective) error {
	for _, backendRef := range slo.Spec.Backends {
		b, err := r.Registry.Get(backendRef.Name)
		if err != nil {
			continue
		}
		if err := b.Cleanup(ctx, slo); err != nil {
			return err
		}
	}
	return nil
}

func (r *ServiceLevelObjectiveReconciler) pruneOrphans(ctx context.Context, slo *slov1alpha1.ServiceLevelObjective, current []slov1alpha1.GeneratedResource) {
	log := logf.FromContext(ctx)
	currentSet := make(map[string]bool)
	for _, res := range current {
		currentSet[res.Backend+"/"+res.Name] = true
	}

	for _, prev := range slo.Status.GeneratedResources {
		key := prev.Backend + "/" + prev.Name
		if !currentSet[key] {
			cm := &corev1.ConfigMap{}
			cm.Name = prev.Name
			cm.Namespace = prev.Namespace
			if err := r.Delete(ctx, cm); err != nil && !errors.IsNotFound(err) {
				log.Error(err, "failed to delete orphan", "name", prev.Name)
			}
		}
	}
}

func (r *ServiceLevelObjectiveReconciler) setCondition(slo *slov1alpha1.ServiceLevelObjective, condType string, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               condType,
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}

	for i, c := range slo.Status.Conditions {
		if c.Type == condType {
			if c.Status != status {
				slo.Status.Conditions[i] = condition
			}
			return
		}
	}
	slo.Status.Conditions = append(slo.Status.Conditions, condition)
}

func (r *ServiceLevelObjectiveReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&slov1alpha1.ServiceLevelObjective{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&monitoringv1.PrometheusRule{}).
		Named("servicelevelobjective").
		Complete(r)
}
