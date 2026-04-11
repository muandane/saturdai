package aggregate

import (
	"testing"
)

func TestUpdate_roundTripSketchAndEMA(t *testing.T) {
	var sketch string
	var short, long float64

	err := Update(ResourceSample{
		Value:     100,
		GetSketch: func() string { return sketch },
		SetSketch: func(v string) { sketch = v },
		GetEMA:    func() (float64, float64) { return short, long },
		SetEMA: func(s, l float64) {
			short, long = s, l
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if sketch == "" {
		t.Fatal("expected encoded sketch")
	}
	if short == 0 && long == 0 {
		t.Fatal("expected EMA update")
	}

	// Second sample: EMA should move from prior
	prevShort, prevLong := short, long
	err = Update(ResourceSample{
		Value:     200,
		GetSketch: func() string { return sketch },
		SetSketch: func(v string) { sketch = v },
		GetEMA:    func() (float64, float64) { return short, long },
		SetEMA: func(s, l float64) {
			short, long = s, l
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if short == prevShort && long == prevLong {
		t.Fatal("expected EMA to change")
	}
}

func TestUpdate_zeroValueStillAddsToSketch(t *testing.T) {
	var sketch string
	var short, long float64

	if err := Update(ResourceSample{
		Value:     0,
		GetSketch: func() string { return sketch },
		SetSketch: func(v string) { sketch = v },
		GetEMA:    func() (float64, float64) { return short, long },
		SetEMA:    func(s, l float64) { short, long = s, l },
	}); err != nil {
		t.Fatal(err)
	}
	if sketch == "" {
		t.Fatal("sketch should be encoded after Add(0)")
	}
	if short != 0 || long != 0 {
		t.Fatalf("EMA should not move on zero sample, got short=%v long=%v", short, long)
	}
}

func TestUpdate_corruptSketchFallsBackToFresh(t *testing.T) {
	var sketch = "not-valid-base64!!!"
	var out string
	err := Update(ResourceSample{
		Value:     50,
		GetSketch: func() string { return sketch },
		SetSketch: func(v string) { out = v },
		GetEMA:    func() (float64, float64) { return 0, 0 },
		SetEMA:    func(s, l float64) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out == "" {
		t.Fatal("expected new sketch")
	}
}
