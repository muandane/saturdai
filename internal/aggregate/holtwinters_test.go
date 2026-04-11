package aggregate

import "testing"

func TestDefaultHWState_seasonOnes(t *testing.T) {
	s := DefaultHWState()
	for i, v := range s.Season {
		if v != 1.0 {
			t.Fatalf("season[%d]=%v", i, v)
		}
	}
}

func TestHWState_Update_warmup(t *testing.T) {
	h := DefaultHWState()
	for i := range 10 {
		_ = h.Update(float64(i), i%24)
	}
	if h.Samples != 10 {
		t.Fatalf("samples=%d", h.Samples)
	}
}
