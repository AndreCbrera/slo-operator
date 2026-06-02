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
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type ServiceLevelObjectiveValidator struct{}

func (v *ServiceLevelObjectiveValidator) ValidateCreate(_ context.Context, slo *ServiceLevelObjective) (admission.Warnings, error) {
	return validateSLO(slo)
}

func (v *ServiceLevelObjectiveValidator) ValidateUpdate(_ context.Context, _ *ServiceLevelObjective, newSLO *ServiceLevelObjective) (admission.Warnings, error) {
	return validateSLO(newSLO)
}

func (v *ServiceLevelObjectiveValidator) ValidateDelete(_ context.Context, _ *ServiceLevelObjective) (admission.Warnings, error) {
	return nil, nil
}

func validateSLO(slo *ServiceLevelObjective) (admission.Warnings, error) {
	var warnings admission.Warnings

	switch slo.Spec.SLI.Type {
	case SLITypeAvailability:
		if slo.Spec.SLI.Availability == nil {
			return nil, fmt.Errorf("sli.availability is required when type is 'availability'")
		}
		if slo.Spec.SLI.Availability.TotalQuery == "" || slo.Spec.SLI.Availability.ErrorQuery == "" {
			return nil, fmt.Errorf("sli.availability.totalQuery and errorQuery are required")
		}
	case SLITypeLatency:
		if slo.Spec.SLI.Latency == nil {
			return nil, fmt.Errorf("sli.latency is required when type is 'latency'")
		}
		if slo.Spec.SLI.Latency.TotalQuery == "" || slo.Spec.SLI.Latency.SuccessQuery == "" {
			return nil, fmt.Errorf("sli.latency.totalQuery and successQuery are required")
		}
		if slo.Spec.SLI.Latency.ThresholdMs <= 0 {
			return nil, fmt.Errorf("sli.latency.thresholdMs must be positive")
		}
	case SLITypeErrorRate, SLITypeThroughput:
		if slo.Spec.SLI.Raw == nil && slo.Spec.SLI.Availability == nil {
			return nil, fmt.Errorf("sli.raw or sli.availability is required for type %q", slo.Spec.SLI.Type)
		}
		if slo.Spec.SLI.Raw != nil {
			if slo.Spec.SLI.Raw.GoodEventsQuery == "" || slo.Spec.SLI.Raw.TotalEventsQuery == "" {
				return nil, fmt.Errorf("sli.raw.goodEventsQuery and totalEventsQuery are required")
			}
		}
	}

	if slo.Spec.Alerting == nil {
		warnings = append(warnings, "no alerting configuration specified, using defaults")
	}

	return warnings, nil
}
