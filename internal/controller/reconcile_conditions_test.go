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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	autosizev1 "github.com/muandane/saturdai/api/v1"
)

func TestSyncProfileReady(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setup      func(*autosizev1.WorkloadProfile)
		wantStatus metav1.ConditionStatus
		wantReason string
		wantMsg    string
	}{
		{
			name: "both_prerequisites_true",
			setup: func(p *autosizev1.WorkloadProfile) {
				setCondition(p, autosizev1.ConditionTypeTargetResolved, metav1.ConditionTrue, "Resolved", "target found")
				setCondition(p, autosizev1.ConditionTypeMetricsAvailable, metav1.ConditionTrue, "Collected", "metrics processed")
			},
			wantStatus: metav1.ConditionTrue,
			wantReason: "Ready",
			wantMsg:    "target resolved and metrics available",
		},
		{
			name: "target_false_metrics_true_prefers_target",
			setup: func(p *autosizev1.WorkloadProfile) {
				setCondition(p, autosizev1.ConditionTypeTargetResolved, metav1.ConditionFalse, "NotFound", "deployment missing")
				setCondition(p, autosizev1.ConditionTypeMetricsAvailable, metav1.ConditionTrue, "Collected", "ok")
			},
			wantStatus: metav1.ConditionFalse,
			wantReason: "TargetNotReady",
			wantMsg:    "deployment missing",
		},
		{
			name: "target_true_metrics_false",
			setup: func(p *autosizev1.WorkloadProfile) {
				setCondition(p, autosizev1.ConditionTypeTargetResolved, metav1.ConditionTrue, "Resolved", "ok")
				setCondition(p, autosizev1.ConditionTypeMetricsAvailable, metav1.ConditionFalse, "KubeletUnavailable", "node-a: timeout")
			},
			wantStatus: metav1.ConditionFalse,
			wantReason: "MetricsNotAvailable",
			wantMsg:    "node-a: timeout",
		},
		{
			name: "both_false_prefers_target",
			setup: func(p *autosizev1.WorkloadProfile) {
				setCondition(p, autosizev1.ConditionTypeTargetResolved, metav1.ConditionFalse, "Error", "api error")
				setCondition(p, autosizev1.ConditionTypeMetricsAvailable, metav1.ConditionFalse, "KubeletUnavailable", "unreachable")
			},
			wantStatus: metav1.ConditionFalse,
			wantReason: "TargetNotReady",
			wantMsg:    "api error",
		},
		{
			name:       "no_conditions_defaults_target_not_ready_message",
			setup:      func(_ *autosizev1.WorkloadProfile) {},
			wantStatus: metav1.ConditionFalse,
			wantReason: "TargetNotReady",
			wantMsg:    "target workload not ready",
		},
		{
			name: "target_true_no_metrics_condition",
			setup: func(p *autosizev1.WorkloadProfile) {
				setCondition(p, autosizev1.ConditionTypeTargetResolved, metav1.ConditionTrue, "Resolved", "ok")
			},
			wantStatus: metav1.ConditionFalse,
			wantReason: "MetricsNotAvailable",
			wantMsg:    "metrics not yet available",
		},
		{
			name: "target_false_empty_message_uses_default",
			setup: func(p *autosizev1.WorkloadProfile) {
				setCondition(p, autosizev1.ConditionTypeTargetResolved, metav1.ConditionFalse, "NotFound", "")
			},
			wantStatus: metav1.ConditionFalse,
			wantReason: "TargetNotReady",
			wantMsg:    "target workload not ready",
		},
		{
			name: "metrics_false_empty_message_uses_default",
			setup: func(p *autosizev1.WorkloadProfile) {
				setCondition(p, autosizev1.ConditionTypeTargetResolved, metav1.ConditionTrue, "Resolved", "ok")
				setCondition(p, autosizev1.ConditionTypeMetricsAvailable, metav1.ConditionFalse, "KubeletUnavailable", "")
			},
			wantStatus: metav1.ConditionFalse,
			wantReason: "MetricsNotAvailable",
			wantMsg:    "metrics not yet available",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := &autosizev1.WorkloadProfile{}
			tt.setup(p)
			syncProfileReady(p)
			got := findCondition(p, autosizev1.ConditionTypeProfileReady)
			if got == nil {
				t.Fatal("ProfileReady condition not set")
			}
			if got.Status != tt.wantStatus {
				t.Errorf("Status = %q, want %q", got.Status, tt.wantStatus)
			}
			if got.Reason != tt.wantReason {
				t.Errorf("Reason = %q, want %q", got.Reason, tt.wantReason)
			}
			if got.Message != tt.wantMsg {
				t.Errorf("Message = %q, want %q", got.Message, tt.wantMsg)
			}
		})
	}
}

func findCondition(p *autosizev1.WorkloadProfile, typ string) *metav1.Condition {
	for i := range p.Status.Conditions {
		if p.Status.Conditions[i].Type == typ {
			return &p.Status.Conditions[i]
		}
	}
	return nil
}
