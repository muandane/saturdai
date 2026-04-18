// Package recommend computes deterministic resource recommendations from sketches and EMAs.
package recommend

import (
	"math"

	"github.com/DataDog/sketches-go/ddsketch"
	"k8s.io/apimachinery/pkg/api/resource"

	autosizev1 "github.com/muandane/saturdai/api/v1"

	"github.com/muandane/saturdai/internal/aggregate"
)

// minQuadrantSketchCount is the minimum DDSketch sample count before quadrant quantiles are trusted over the global sketch.
const minQuadrantSketchCount = 30.0

func sketchHasEnoughSamples(sk *ddsketch.DDSketch) bool {
	return sk != nil && !sk.IsEmpty() && sk.GetCount() >= minQuadrantSketchCount
}

// SchedulerBalanceUnknown means no cluster balance observation; skip packed-cluster request inflation.
const SchedulerBalanceUnknown = -1.0

// Input carries per-container metrics for recommendation.
type Input struct {
	ContainerName string
	Mode          string
	CPUSketch     *ddsketch.DDSketch
	MemSketch     *ddsketch.DDSketch
	// QuadrantCPUSketch optional time-of-day sketch (UTC 6h bucket); non-empty overrides CPUSketch for quantiles.
	QuadrantCPUSketch *ddsketch.DDSketch
	QuadrantMemSketch *ddsketch.DDSketch
	CPUEShort         float64
	CPUELong          float64
	MemShort          float64
	MemLong           float64
	// ForecastCPU, ForecastMem optional Holt-Winters forecasts (millicores / bytes); used when > 0.
	ForecastCPU float64
	ForecastMem float64
	MinCPU      *resource.Quantity
	MaxCPU      *resource.Quantity
	MinMemory   *resource.Quantity
	MaxMemory   *resource.Quantity
	// SchedulerBalanceScore is mean free-allocatable share [0,1] across nodes hosting the workload (scheduler-adjacent).
	// -1 means unknown / do not apply packed-cluster adjustment.
	SchedulerBalanceScore float64
}

func effectiveCPUSketch(in Input) *ddsketch.DDSketch {
	if sk := in.QuadrantCPUSketch; sketchHasEnoughSamples(sk) {
		return sk
	}
	return in.CPUSketch
}

func effectiveMemSketch(in Input) *ddsketch.DDSketch {
	if sk := in.QuadrantMemSketch; sketchHasEnoughSamples(sk) {
		return sk
	}
	return in.MemSketch
}

func memShortForBurst(in Input) float64 {
	if in.ForecastMem > 0 {
		return math.Max(in.MemShort, in.ForecastMem)
	}
	return in.MemShort
}

// Compute returns a recommendation for one container (no bias).
func Compute(in Input) (autosizev1.Recommendation, error) {
	return New(in.Mode, NoopBias{}).Compute(in)
}

func normalizeMode(mode string) string {
	if mode == "" {
		return "balanced"
	}
	return mode
}

func qMilli(sk *ddsketch.DDSketch, q float64) (float64, error) {
	if sk == nil || sk.IsEmpty() {
		return 0, nil
	}
	v, err := aggregate.Quantile(sk, q)
	if err != nil {
		return 0, nil
	}
	return v, nil
}

func qBytes(sk *ddsketch.DDSketch, q float64) (float64, error) {
	return qMilli(sk, q)
}

func milliQty(m float64) resource.Quantity {
	if m < 0 {
		m = 0
	}
	return *resource.NewMilliQuantity(int64(math.Round(m)), resource.DecimalSI)
}

func bytesQty(b float64) resource.Quantity {
	if b < 0 {
		b = 0
	}
	return *resource.NewQuantity(int64(math.Round(b)), resource.BinarySI)
}

func clampQty(q resource.Quantity, minQ, maxQ *resource.Quantity) resource.Quantity {
	out := q
	if minQ != nil && out.Cmp(*minQ) < 0 {
		out = *minQ
	}
	if maxQ != nil && out.Cmp(*maxQ) > 0 {
		out = *maxQ
	}
	return out
}
