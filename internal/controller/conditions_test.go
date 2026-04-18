package controller

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	autosizev1 "github.com/muandane/saturdai/api/v1"
)

func TestSetCSPCondition_preservesTransitionTimeWhenStatusUnchanged(t *testing.T) {
	profile := &autosizev1.ClusterProfile{}
	setCSPCondition(profile, autosizev1.ConditionTypeChildrenSynced, metav1.ConditionTrue, "Synced", "first")
	if len(profile.Status.Conditions) != 1 {
		t.Fatalf("conditions: got %d want 1", len(profile.Status.Conditions))
	}
	first := profile.Status.Conditions[0].LastTransitionTime
	setCSPCondition(profile, autosizev1.ConditionTypeChildrenSynced, metav1.ConditionTrue, "StillSynced", "second")
	second := profile.Status.Conditions[0].LastTransitionTime
	if !first.Equal(&second) {
		t.Fatalf("LastTransitionTime changed: first=%v second=%v", first.Time, second.Time)
	}
}

func TestSetNSPCondition_preservesTransitionTimeWhenStatusUnchanged(t *testing.T) {
	profile := &autosizev1.NamespaceProfile{}
	setNSPCondition(profile, autosizev1.ConditionTypeChildrenSynced, metav1.ConditionFalse, "NoTargets", "first")
	if len(profile.Status.Conditions) != 1 {
		t.Fatalf("conditions: got %d want 1", len(profile.Status.Conditions))
	}
	first := profile.Status.Conditions[0].LastTransitionTime
	setNSPCondition(profile, autosizev1.ConditionTypeChildrenSynced, metav1.ConditionFalse, "StillNoTargets", "second")
	second := profile.Status.Conditions[0].LastTransitionTime
	if !first.Equal(&second) {
		t.Fatalf("LastTransitionTime changed: first=%v second=%v", first.Time, second.Time)
	}
}

func TestSetCondition_preservesTransitionTimeWhenStatusUnchanged(t *testing.T) {
	profile := &autosizev1.WorkloadProfile{}
	setCondition(profile, autosizev1.ConditionTypeMetricsAvailable, metav1.ConditionTrue, "Collected", "first")
	if len(profile.Status.Conditions) != 1 {
		t.Fatalf("conditions: got %d want 1", len(profile.Status.Conditions))
	}
	first := profile.Status.Conditions[0].LastTransitionTime
	setCondition(profile, autosizev1.ConditionTypeMetricsAvailable, metav1.ConditionTrue, "CollectedAgain", "second")
	second := profile.Status.Conditions[0].LastTransitionTime
	if !first.Equal(&second) {
		t.Fatalf("LastTransitionTime changed: first=%v second=%v", first.Time, second.Time)
	}
}
