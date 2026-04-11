package controller

import "testing"

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
