package controller

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/muandane/saturdai/internal/actuate"
)

var (
	registerActuationMetricsOnce sync.Once

	actuationTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "autosize_actuation_total",
			Help: "Total actuation outcomes for in-place pod resizing.",
		},
		[]string{"result"},
	)

	actuationResizeReasonTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "autosize_actuation_pod_resize_reason_total",
			Help: "Total resize reason bucket observations from failed pod resize attempts.",
		},
		[]string{"reason"},
	)
)

func ensureActuationMetricsRegistered() {
	registerActuationMetricsOnce.Do(func() {
		ctrlmetrics.Registry.MustRegister(actuationTotal, actuationResizeReasonTotal)
	})
}

func observeActuationMetrics(result actuate.Result, err error) {
	ensureActuationMetricsRegistered()

	switch {
	case err != nil:
		actuationTotal.WithLabelValues("error").Inc()
	case result.Resized == 0:
		actuationTotal.WithLabelValues("noop").Inc()
	default:
		actuationTotal.WithLabelValues("success").Inc()
	}

	for reason, count := range result.ReasonCounts {
		if count > 0 {
			actuationResizeReasonTotal.WithLabelValues(reason).Add(float64(count))
		}
	}
	if result.RestartPolicyWarnings > 0 {
		actuationResizeReasonTotal.WithLabelValues("restart_policy_requires_restart").Add(float64(result.RestartPolicyWarnings))
	}
}
