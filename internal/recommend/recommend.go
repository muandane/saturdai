// Package recommend computes deterministic resource recommendations from sketches and EMAs.
package recommend

import (
	"fmt"
	"math"

	"github.com/DataDog/sketches-go/ddsketch"
	"k8s.io/apimachinery/pkg/api/resource"

	autosizev1 "github.com/muandane/saturdai/api/v1"

	"github.com/muandane/saturdai/internal/aggregate"
)

// Input carries per-container metrics for recommendation.
type Input struct {
	ContainerName string
	Mode          string
	CPUSketch     *ddsketch.DDSketch
	MemSketch     *ddsketch.DDSketch
	CPUEShort     float64
	CPUELong      float64
	MemShort      float64
	MemLong       float64
	MinCPU        *resource.Quantity
	MaxCPU        *resource.Quantity
	MinMemory     *resource.Quantity
	MaxMemory     *resource.Quantity
}

// Compute returns a recommendation for one container.
func Compute(in Input) (autosizev1.Recommendation, error) {
	mode := normalizeMode(in.Mode)
	var cpuReqMilli, cpuLimMilli, memReqBytes, memLimBytes float64
	var rationale string

	switch mode {
	case "cost":
		cpuReqMilli, _ = qMilli(in.CPUSketch, 0.50)
		cpuLimMilli, _ = qMilli(in.CPUSketch, 0.90)
		memReqBytes, _ = qBytes(in.MemSketch, 0.50)
		memLimBytes, _ = qBytes(in.MemSketch, 0.90)
		rationale = fmt.Sprintf("cost: P50/P90 cpu & mem, mode=%s", mode)
	case "balanced":
		cpuReqMilli, _ = qMilli(in.CPUSketch, 0.70)
		cpuLimMilli, _ = qMilli(in.CPUSketch, 0.95)
		memReqBytes, _ = qBytes(in.MemSketch, 0.70)
		memLimBytes, _ = qBytes(in.MemSketch, 0.95)
		rationale = fmt.Sprintf("balanced: P70/P95 cpu & mem, mode=%s", mode)
	case "resilience":
		p90c, _ := qMilli(in.CPUSketch, 0.90)
		p99c, _ := qMilli(in.CPUSketch, 0.99)
		cpuReqMilli = p90c
		cpuLimMilli = p99c * 1.1
		p90m, _ := qBytes(in.MemSketch, 0.90)
		p99m, _ := qBytes(in.MemSketch, 0.99)
		memReqBytes = p90m
		memLimBytes = p99m * 1.2
		rationale = fmt.Sprintf("resilience: P90 req, P99*1.1/1.2 limits, mode=%s", mode)
	case "burst":
		p40c, _ := qMilli(in.CPUSketch, 0.40)
		p99c, _ := qMilli(in.CPUSketch, 0.99)
		cpuReqMilli = p40c
		peakCPU := math.Max(p99c, in.CPUEShort)
		cpuLimMilli = peakCPU
		p40m, _ := qBytes(in.MemSketch, 0.40)
		memReqBytes = p40m
		p99m, _ := qBytes(in.MemSketch, 0.99)
		peakMem := math.Max(p99m, in.MemShort)
		memLimBytes = peakMem
		rationale = fmt.Sprintf("burst: P40 req, peak=max(P99,EMA_short), mode=%s", mode)
	default:
		cpuReqMilli, _ = qMilli(in.CPUSketch, 0.70)
		cpuLimMilli, _ = qMilli(in.CPUSketch, 0.95)
		memReqBytes, _ = qBytes(in.MemSketch, 0.70)
		memLimBytes, _ = qBytes(in.MemSketch, 0.95)
		rationale = fmt.Sprintf("balanced(default): P70/P95, mode=%s", mode)
	}

	cpuReq := milliQty(cpuReqMilli)
	cpuLim := milliQty(cpuLimMilli)
	memReq := bytesQty(memReqBytes)
	memLim := bytesQty(memLimBytes)

	cpuReq = clampQty(cpuReq, in.MinCPU, in.MaxCPU)
	cpuLim = clampQty(cpuLim, in.MinCPU, in.MaxCPU)
	memReq = clampQty(memReq, in.MinMemory, in.MaxMemory)
	memLim = clampQty(memLim, in.MinMemory, in.MaxMemory)

	return autosizev1.Recommendation{
		ContainerName: in.ContainerName,
		CPURequest:    cpuReq,
		CPULimit:      cpuLim,
		MemoryRequest: memReq,
		MemoryLimit:   memLim,
		Rationale:     rationale,
	}, nil
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
	return qMilli(sk, q) // same API — values are in bytes for memory sketch
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
