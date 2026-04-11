package changepoint

import "testing"

func TestState_Update_detectsPositiveShift(t *testing.T) {
	var s State
	cfg := Config{K: 1, H: 5}
	target := 10.0
	// drift high
	shifted := false
	for range 20 {
		if s.Update(50, target, cfg) {
			shifted = true
			break
		}
	}
	if !shifted {
		t.Fatal("expected positive shift detection")
	}
}

func TestState_Reset(t *testing.T) {
	var s State
	s.SPos, s.SNeg = 3, 4
	s.Reset()
	if s.SPos != 0 || s.SNeg != 0 {
		t.Fatalf("got %+v", s)
	}
}
