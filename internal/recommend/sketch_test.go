package recommend

import (
	"testing"

	"github.com/DataDog/sketches-go/ddsketch"
)

func TestSketchHasEnoughSamples(t *testing.T) {
	sk, err := ddsketch.NewDefaultDDSketch(0.01)
	if err != nil {
		t.Fatal(err)
	}
	if sketchHasEnoughSamples(sk) {
		t.Fatal("empty sketch should not pass")
	}
	for range 29 {
		if err := sk.Add(1); err != nil {
			t.Fatal(err)
		}
	}
	if sketchHasEnoughSamples(sk) {
		t.Fatal("29 samples should not pass")
	}
	if err := sk.Add(1); err != nil {
		t.Fatal(err)
	}
	if !sketchHasEnoughSamples(sk) {
		t.Fatal("30 samples should pass")
	}
}

func TestEffectiveCPUSketch_prefersQuadrantWhenEnoughSamples(t *testing.T) {
	global, err := ddsketch.NewDefaultDDSketch(0.01)
	if err != nil {
		t.Fatal(err)
	}
	_ = global.Add(100)
	quad, err := ddsketch.NewDefaultDDSketch(0.01)
	if err != nil {
		t.Fatal(err)
	}
	for range 30 {
		if err := quad.Add(5); err != nil {
			t.Fatal(err)
		}
	}
	in := Input{CPUSketch: global, QuadrantCPUSketch: quad, SchedulerBalanceScore: SchedulerBalanceUnknown}
	if effectiveCPUSketch(in) != quad {
		t.Fatal("expected quadrant when count sufficient")
	}
}
