package controller

import "testing"

func Test_isRestartSpike(t *testing.T) {
	if isRestartSpike(0, 10, false) {
		t.Fatal("without baseline must not spike")
	}
	if !isRestartSpike(0, 10, true) {
		t.Fatal("delta 10 > 3 should spike")
	}
	if isRestartSpike(5, 8, true) {
		t.Fatal("delta 3 is not a spike")
	}
	if !isRestartSpike(2, 6, true) {
		t.Fatal("delta 4 should spike")
	}
}

func Test_restartPauseAfterReconcile(t *testing.T) {
	tests := []struct {
		name           string
		baselineSeen   bool
		anySpike       bool
		pauseRemaining int32
		want           int32
	}{
		{"spike sets 2", true, true, 0, 2},
		{"spike resets from 1 to 2", true, true, 1, 2},
		{"no baseline spike does not set", false, true, 0, 0},
		{"decrement when no spike", true, false, 2, 1},
		{"decrement to zero", true, false, 1, 0},
		{"stays zero", true, false, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := restartPauseAfterReconcile(tt.baselineSeen, tt.anySpike, tt.pauseRemaining)
			if got != tt.want {
				t.Fatalf("got %d want %d", got, tt.want)
			}
		})
	}
}
