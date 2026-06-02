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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:validation:Enum=availability;latency;throughput;error_rate
type SLIType string

const (
	SLITypeAvailability SLIType = "availability"
	SLITypeLatency      SLIType = "latency"
	SLITypeThroughput   SLIType = "throughput"
	SLITypeErrorRate    SLIType = "error_rate"
)

// +kubebuilder:validation:Enum="7d";"14d";"28d";"30d";"90d"
type Window string

const (
	Window7d  Window = "7d"
	Window14d Window = "14d"
	Window28d Window = "28d"
	Window30d Window = "30d"
	Window90d Window = "90d"
)

type ServiceLevelObjectiveSpec struct {
	// +kubebuilder:validation:MinLength=1
	Service string `json:"service"`

	// +optional
	Description string `json:"description,omitempty"`

	SLI SLISpec `json:"sli"`

	// +kubebuilder:validation:Pattern=`^\d{1,3}(\.\d{1,4})?$`
	Target string `json:"target"`

	Window Window `json:"window"`

	// +optional
	Alerting *AlertingSpec `json:"alerting,omitempty"`

	// +kubebuilder:validation:MinItems=1
	Backends []BackendRef `json:"backends"`
}

type SLISpec struct {
	Type SLIType `json:"type"`

	// +optional
	Availability *AvailabilitySLI `json:"availability,omitempty"`

	// +optional
	Latency *LatencySLI `json:"latency,omitempty"`

	// +optional
	Raw *RawSLI `json:"raw,omitempty"`
}

type AvailabilitySLI struct {
	TotalQuery string `json:"totalQuery"`
	ErrorQuery string `json:"errorQuery"`
}

type LatencySLI struct {
	TotalQuery   string `json:"totalQuery"`
	SuccessQuery string `json:"successQuery"`
	// +kubebuilder:validation:Minimum=1
	ThresholdMs int64 `json:"thresholdMs"`
}

type RawSLI struct {
	GoodEventsQuery  string `json:"goodEventsQuery"`
	TotalEventsQuery string `json:"totalEventsQuery"`
}

type AlertingSpec struct {
	// +optional
	Disabled bool `json:"disabled,omitempty"`

	// +optional
	PageBurnRate *string `json:"pageBurnRate,omitempty"`

	// +optional
	TicketBurnRate *string `json:"ticketBurnRate,omitempty"`

	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// +kubebuilder:validation:Enum=prometheus;grafana;datadog;cloudwatch
type BackendName string

const (
	BackendPrometheus  BackendName = "prometheus"
	BackendGrafana     BackendName = "grafana"
	BackendDatadog     BackendName = "datadog"
	BackendCloudWatch  BackendName = "cloudwatch"
)

type BackendRef struct {
	Name BackendName `json:"name"`

	// +optional
	Config map[string]string `json:"config,omitempty"`
}

type ServiceLevelObjectiveStatus struct {
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +optional
	GeneratedResources []GeneratedResource `json:"generatedResources,omitempty"`

	// +optional
	LastReconcileTime *metav1.Time `json:"lastReconcileTime,omitempty"`
}

type GeneratedResource struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Backend   string `json:"backend"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Service",type=string,JSONPath=`.spec.service`
// +kubebuilder:printcolumn:name="Target",type=string,JSONPath=`.spec.target`
// +kubebuilder:printcolumn:name="Window",type=string,JSONPath=`.spec.window`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type ServiceLevelObjective struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec ServiceLevelObjectiveSpec `json:"spec"`

	// +optional
	Status ServiceLevelObjectiveStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true
type ServiceLevelObjectiveList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []ServiceLevelObjective `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ServiceLevelObjective{}, &ServiceLevelObjectiveList{})
}
