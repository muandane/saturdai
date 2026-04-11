package aggregate

import (
	"math"
	"testing"
)

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

func TestHWState_Update_warmupIncrementalMean(t *testing.T) {
	h := DefaultHWState()
	_ = h.Update(10, 12)
	_ = h.Update(30, 13)
	if h.Level != 20 {
		t.Fatalf("expected level 20 after two warmup samples, got %v", h.Level)
	}
	if h.Samples != 2 {
		t.Fatalf("samples=%d", h.Samples)
	}
}

func TestHWState_Update_noNaNSeasonWhenLevelAndSampleZero(t *testing.T) {
	h := DefaultHWState()
	for i := range 24 {
		_ = h.Update(0, i%24)
	}
	for range 30 {
		out := h.Update(0, 0)
		if math.IsNaN(out) || math.IsInf(out, 0) {
			t.Fatalf("non-finite forecast %v", out)
		}
		for _, v := range h.Season {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				t.Fatalf("non-finite season %v", v)
			}
		}
		if math.IsNaN(h.Level) || math.IsInf(h.Level, 0) {
			t.Fatalf("non-finite level %v", h.Level)
		}
	}
}

func TestHWState_Update_ignoresNaNSample(t *testing.T) {
	h := DefaultHWState()
	out := h.Update(math.NaN(), 0)
	if math.IsNaN(out) || math.IsInf(out, 0) {
		t.Fatalf("warmup should return finite, got %v", out)
	}
}
