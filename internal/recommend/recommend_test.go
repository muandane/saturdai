package recommend

import (
	"strings"
	"testing"

	"github.com/DataDog/sketches-go/ddsketch"
)

func TestKForMode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		mode string
		want float64
	}{
		{"", 1.0},
		{"balanced", 1.0},
		{"cost", 0.5},
		{"resilience", 1.5},
		{"burst", 2.0},
		{"unknown-mode", 1.0},
	}
	for _, tc := range cases {
		if g := kForMode(tc.mode); g != tc.want {
			t.Errorf("kForMode(%q) = %v, want %v", tc.mode, g, tc.want)
		}
	}
}

func TestEmaPrediction(t *testing.T) {
	t.Parallel()
	// EMA_long + k*(EMA_short - EMA_long)
	if g := emaPrediction(200, 100, 1.0); g != 200 {
		t.Fatalf("emaPrediction got %v want 200", g)
	}
	if g := emaPrediction(200, 100, 0.5); g != 150 {
		t.Fatalf("emaPrediction got %v want 150", g)
	}
}

func TestLimitWithPrediction(t *testing.T) {
	t.Parallel()
	if g := limitWithPrediction(500, 200, 100, 1.0); g != 500 {
		t.Fatalf("quantile wins: got %v want 500", g)
	}
	if g := limitWithPrediction(500, 800, 100, 1.0); g != 800 {
		t.Fatalf("prediction wins: got %v want 800", g)
	}
}

func sketchConstant(val float64) *ddsketch.DDSketch {
	const samples = 200
	sk, _ := ddsketch.NewDefaultDDSketch(0.01)
	for range samples {
		_ = sk.Add(val)
	}
	return sk
}

func TestCompute_PerMode_KInRationale(t *testing.T) {
	t.Parallel()
	modes := []struct {
		mode string
		k    string // substring in rationale
	}{
		{"cost", "k=0.5"},
		{"balanced", "k=1.0"},
		{"resilience", "k=1.5"},
		{"burst", "k=2.0"},
	}
	sk := sketchConstant(1000)
	for _, tc := range modes {
		in := Input{
			ContainerName: "app",
			Mode:          tc.mode,
			CPUSketch:     sk,
			MemSketch:     sk,
			CPUEShort:     1000,
			CPUELong:      1000,
			MemShort:      1000,
			MemLong:       1000,
		}
		rec, err := Compute(in)
		if err != nil {
			t.Fatalf("%s: %v", tc.mode, err)
		}
		if !strings.Contains(rec.Rationale, tc.k) {
			t.Fatalf("%s: rationale %q missing %q", tc.mode, rec.Rationale, tc.k)
		}
		if !strings.Contains(rec.Rationale, "cpu_pred=") || !strings.Contains(rec.Rationale, "mem_pred=") {
			t.Fatalf("%s: rationale should mention cpu_pred and mem_pred: %q", tc.mode, rec.Rationale)
		}
	}
}

func TestCompute_PredictionRaisesCPULimitAboveQuantile(t *testing.T) {
	t.Parallel()
	// Quantiles ~1000m; EMAs imply higher prediction for balanced (k=1).
	sk := sketchConstant(1000)
	in := Input{
		ContainerName: "app",
		Mode:          "balanced",
		CPUSketch:     sk,
		MemSketch:     sk,
		CPUEShort:     5000,
		CPUELong:      100,
		MemShort:      1000,
		MemLong:       1000,
	}
	rec, err := Compute(in)
	if err != nil {
		t.Fatal(err)
	}
	// P95 ~1000; pred cpu = 100 + 1*(5000-100) = 5000
	wantLim := int64(5000)
	if rec.CPULimit.MilliValue() != wantLim {
		t.Fatalf("CPULimit %s want %dm", rec.CPULimit.String(), wantLim)
	}
}

func TestMonotonicity_HigherShortEMARaisesCPULimit(t *testing.T) {
	t.Parallel()
	sk := sketchConstant(400)
	base := Input{
		ContainerName: "app",
		Mode:          "balanced",
		CPUSketch:     sk,
		MemSketch:     sk,
		CPUELong:      100,
		MemShort:      400,
		MemLong:       100,
	}
	low := base
	low.CPUEShort = 200
	high := base
	high.CPUEShort = 900

	rLow, err := Compute(low)
	if err != nil {
		t.Fatal(err)
	}
	rHigh, err := Compute(high)
	if err != nil {
		t.Fatal(err)
	}
	// Quantile P95 ~400; pred low = 200 -> max(400,200)=400; pred high = 900 -> max(400,900)=900
	if rLow.CPULimit.MilliValue() >= rHigh.CPULimit.MilliValue() {
		t.Fatalf("expected higher short EMA to raise limit: low=%s high=%s", rLow.CPULimit.String(), rHigh.CPULimit.String())
	}
}

func TestCompute_Cost_UsesHalfK(t *testing.T) {
	t.Parallel()
	sk := sketchConstant(500)
	in := Input{
		ContainerName: "app",
		Mode:          "cost",
		CPUSketch:     sk,
		MemSketch:     sk,
		CPUEShort:     3000,
		CPUELong:      100,
		MemShort:      500,
		MemLong:       500,
	}
	rec, err := Compute(in)
	if err != nil {
		t.Fatal(err)
	}
	// P90 ~500; pred = 100 + 0.5*(2900) = 1550
	want := int64(1550)
	if rec.CPULimit.MilliValue() != want {
		t.Fatalf("CPULimit %s want %dm", rec.CPULimit.String(), want)
	}
}

func TestCompute_Burst_MergesPeakAndPrediction(t *testing.T) {
	t.Parallel()
	sk := sketchConstant(100)
	in := Input{
		ContainerName: "app",
		Mode:          "burst",
		CPUSketch:     sk,
		MemSketch:     sk,
		CPUEShort:     50,
		CPUELong:      0,
		MemShort:      50,
		MemLong:       0,
	}
	rec, err := Compute(in)
	if err != nil {
		t.Fatal(err)
	}
	// peak CPU = max(P99~100, EMA_short 50) = 100; pred = 0 + 2*(50-0)=100 -> max(100,100)=100
	if rec.CPULimit.MilliValue() != 100 {
		t.Fatalf("CPULimit %s want 100m", rec.CPULimit.String())
	}
	// Raise short EMA so prediction beats peak
	in.CPUEShort = 200
	rec2, err := Compute(in)
	if err != nil {
		t.Fatal(err)
	}
	// peak = max(100,200)=200; pred = 0+2*200=400 -> 400
	if rec2.CPULimit.MilliValue() != 400 {
		t.Fatalf("CPULimit %s want 400m", rec2.CPULimit.String())
	}
}
