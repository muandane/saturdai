package controller

import (
	"math"
	"testing"
)

func TestLeastAllocatedScore(t *testing.T) {
	t.Parallel()
	t.Run("empty", func(t *testing.T) {
		if g := leastAllocatedScore(nil); g != 0 {
			t.Fatalf("got %v want 0", g)
		}
		if g := leastAllocatedScore(map[string]nodeSchedulerState{}); g != 0 {
			t.Fatalf("got %v want 0", g)
		}
	})
	t.Run("single_node_half_free", func(t *testing.T) {
		st := map[string]nodeSchedulerState{
			"n1": {AllocatableCPUMilli: 1000, AllocatableMemBytes: 1000, RequestedCPUMilli: 500, RequestedMemBytes: 500},
		}
		want := (0.5 + 0.5) / 2
		if g := leastAllocatedScore(st); math.Abs(g-want) > 1e-9 {
			t.Fatalf("got %v want %v", g, want)
		}
	})
	t.Run("skips_zero_allocatable", func(t *testing.T) {
		st := map[string]nodeSchedulerState{
			"bad": {AllocatableCPUMilli: 0, AllocatableMemBytes: 1000, RequestedCPUMilli: 0, RequestedMemBytes: 0},
			"ok":  {AllocatableCPUMilli: 1000, AllocatableMemBytes: 1000, RequestedCPUMilli: 0, RequestedMemBytes: 0},
		}
		if g := leastAllocatedScore(st); g != 1.0 {
			t.Fatalf("got %v want 1", g)
		}
	})
	t.Run("heterogeneous_nodes_mean", func(t *testing.T) {
		st := map[string]nodeSchedulerState{
			"a": {AllocatableCPUMilli: 1000, AllocatableMemBytes: 1000, RequestedCPUMilli: 0, RequestedMemBytes: 0},
			"b": {AllocatableCPUMilli: 1000, AllocatableMemBytes: 1000, RequestedCPUMilli: 500, RequestedMemBytes: 500},
		}
		// each node score = (1+1)/2=1 and (0.5+0.5)/2=0.5 -> mean 0.75
		want := 0.75
		if g := leastAllocatedScore(st); math.Abs(g-want) > 1e-9 {
			t.Fatalf("got %v want %v", g, want)
		}
	})
}

func TestNodePressureLabel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		score float64
		want  string
	}{
		{0.5, "low"},
		{0.41, "low"},
		{0.4, "medium"},
		{0.2, "medium"},
		{0.15, "high"},
		{0.14, "high"},
		{0, "high"},
	}
	for _, tc := range cases {
		if g := nodePressureLabel(tc.score); g != tc.want {
			t.Errorf("nodePressureLabel(%v) = %q want %q", tc.score, g, tc.want)
		}
	}
}
