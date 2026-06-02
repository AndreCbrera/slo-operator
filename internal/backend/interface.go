package backend

import (
	"context"
	"fmt"

	v1alpha1 "github.com/AndreCbrera/slo-operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Result struct {
	Object     client.Object
	MutateFunc func() error
}

type Backend interface {
	Name() string
	Reconcile(ctx context.Context, slo *v1alpha1.ServiceLevelObjective) ([]Result, error)
	Cleanup(ctx context.Context, slo *v1alpha1.ServiceLevelObjective) error
}

type Registry struct {
	backends map[v1alpha1.BackendName]Backend
}

func NewRegistry() *Registry {
	return &Registry{backends: make(map[v1alpha1.BackendName]Backend)}
}

func (r *Registry) Register(b Backend, name v1alpha1.BackendName) {
	r.backends[name] = b
}

func (r *Registry) Get(name v1alpha1.BackendName) (Backend, error) {
	b, ok := r.backends[name]
	if !ok {
		return nil, fmt.Errorf("backend %q not registered", name)
	}
	return b, nil
}
