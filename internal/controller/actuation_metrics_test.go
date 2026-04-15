package controller

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/muandane/saturdai/internal/actuate"
)

func TestObserveActuationMetrics_ResultBuckets(t *testing.T) {
	ensureActuationMetricsRegistered()

	beforeSuccess := testutil.ToFloat64(actuationTotal.WithLabelValues("success"))
	beforeNoop := testutil.ToFloat64(actuationTotal.WithLabelValues("noop"))
	beforeErr := testutil.ToFloat64(actuationTotal.WithLabelValues("error"))

	observeActuationMetrics(actuate.Result{Resized: 1}, nil)
	observeActuationMetrics(actuate.Result{Noop: 1}, nil)
	observeActuationMetrics(actuate.Result{Failed: 1}, assertErr{})

	if got := testutil.ToFloat64(actuationTotal.WithLabelValues("success")); got != beforeSuccess+1 {
		t.Fatalf("success counter got %v want %v", got, beforeSuccess+1)
	}
	if got := testutil.ToFloat64(actuationTotal.WithLabelValues("noop")); got != beforeNoop+1 {
		t.Fatalf("noop counter got %v want %v", got, beforeNoop+1)
	}
	if got := testutil.ToFloat64(actuationTotal.WithLabelValues("error")); got != beforeErr+1 {
		t.Fatalf("error counter got %v want %v", got, beforeErr+1)
	}
}

func TestObserveActuationMetrics_ReasonBuckets(t *testing.T) {
	ensureActuationMetricsRegistered()
	beforeInfeasible := testutil.ToFloat64(actuationResizeReasonTotal.WithLabelValues("infeasible"))
	beforeRestart := testutil.ToFloat64(actuationResizeReasonTotal.WithLabelValues("restart_policy_requires_restart"))

	observeActuationMetrics(actuate.Result{
		ReasonCounts:          map[string]int{"infeasible": 2},
		RestartPolicyWarnings: 1,
	}, nil)

	if got := testutil.ToFloat64(actuationResizeReasonTotal.WithLabelValues("infeasible")); got != beforeInfeasible+2 {
		t.Fatalf("infeasible reason counter got %v want %v", got, beforeInfeasible+2)
	}
	if got := testutil.ToFloat64(actuationResizeReasonTotal.WithLabelValues("restart_policy_requires_restart")); got != beforeRestart+1 {
		t.Fatalf("restart reason counter got %v want %v", got, beforeRestart+1)
	}
}

type assertErr struct{}

func (assertErr) Error() string { return "err" }
