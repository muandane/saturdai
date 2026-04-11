package controller

import (
	"testing"
	"time"

	autosizev1 "github.com/muandane/saturdai/api/v1"
)

func TestMetricsRequeueAfter(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		spec autosizev1.WorkloadProfileSpec
		want time.Duration
	}{
		{
			name: "default_interval_30s",
			spec: autosizev1.WorkloadProfileSpec{},
			want: 30 * time.Second,
		},
		{
			name: "short_interval_clamped_to_10s",
			spec: autosizev1.WorkloadProfileSpec{
				CollectionIntervalSeconds: int32Ptr(5),
			},
			want: 10 * time.Second,
		},
		{
			name: "long_interval_120s",
			spec: autosizev1.WorkloadProfileSpec{
				CollectionIntervalSeconds: int32Ptr(120),
			},
			want: 120 * time.Second,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := &autosizev1.WorkloadProfile{Spec: tc.spec}
			if got := metricsRequeueAfter(p); got != tc.want {
				t.Fatalf("metricsRequeueAfter() = %v, want %v", got, tc.want)
			}
		})
	}
}

//go:fix inline
func int32Ptr(v int32) *int32 {
	return new(v)
}

func TestKubeletSummariesFullyUnavailable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		nodes     int
		summaries int
		want      bool
	}{
		{"no_nodes", 0, 0, false},
		{"no_nodes_partial", 0, 1, false},
		{"all_fetch_failed", 1, 0, true},
		{"two_nodes_all_failed", 2, 0, true},
		{"partial_ok", 2, 1, false},
		{"all_ok", 2, 2, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := kubeletSummariesFullyUnavailable(tc.nodes, tc.summaries); got != tc.want {
				t.Fatalf("kubeletSummariesFullyUnavailable(%d,%d) = %v, want %v", tc.nodes, tc.summaries, got, tc.want)
			}
		})
	}
}
