package controller

import "testing"

func TestFormatReasonCounts(t *testing.T) {
	got := formatReasonCounts(map[string]int{"deferred": 2, "infeasible": 1})
	if got != "deferred:2,infeasible:1" {
		t.Fatalf("got %q", got)
	}
	if empty := formatReasonCounts(nil); empty != "" {
		t.Fatalf("expected empty, got %q", empty)
	}
}

func TestFormatRestartPolicyWarning(t *testing.T) {
	if got := formatRestartPolicyWarning(0); got != "" {
		t.Fatalf("expected empty for zero, got %q", got)
	}
	if got := formatRestartPolicyWarning(3); got != "; restart_policy_requires_restart=3" {
		t.Fatalf("unexpected warning %q", got)
	}
}
