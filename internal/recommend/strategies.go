package recommend

import (
	"fmt"
	"math"

	autosizev1 "github.com/muandane/saturdai/api/v1"
)

type costStrategy struct{}

func (costStrategy) Compute(in Input) (autosizev1.Recommendation, error) {
	cpuReqMilli, _ := qMilli(effectiveCPUSketch(in), 0.50)
	cpuLimMilli, _ := qMilli(effectiveCPUSketch(in), 0.90)
	memReqBytes, _ := qBytes(effectiveMemSketch(in), 0.50)
	memLimBytes, _ := qBytes(effectiveMemSketch(in), 0.90)
	rationale := fmt.Sprintf("cost: P50/P90 cpu & mem, mode=%s", "cost")
	return finalizeRec(in, cpuReqMilli, cpuLimMilli, memReqBytes, memLimBytes, rationale), nil
}

type balancedStrategy struct {
	fallback bool // true when original mode was unknown (maps to balanced behavior)
}

func (b balancedStrategy) Compute(in Input) (autosizev1.Recommendation, error) {
	mode := normalizeMode(in.Mode)
	cpuReqMilli, _ := qMilli(effectiveCPUSketch(in), 0.70)
	cpuLimMilli, _ := qMilli(effectiveCPUSketch(in), 0.95)
	memReqBytes, _ := qBytes(effectiveMemSketch(in), 0.70)
	memLimBytes, _ := qBytes(effectiveMemSketch(in), 0.95)
	var rationale string
	if b.fallback {
		rationale = fmt.Sprintf("balanced(default): P70/P95, mode=%s", mode)
	} else {
		rationale = fmt.Sprintf("balanced: P70/P95 cpu & mem, mode=%s", mode)
	}
	return finalizeRec(in, cpuReqMilli, cpuLimMilli, memReqBytes, memLimBytes, rationale), nil
}

type resilienceStrategy struct{}

func (resilienceStrategy) Compute(in Input) (autosizev1.Recommendation, error) {
	cpuSk := effectiveCPUSketch(in)
	memSk := effectiveMemSketch(in)
	p90c, _ := qMilli(cpuSk, 0.90)
	p99c, _ := qMilli(cpuSk, 0.99)
	cpuReqMilli := p90c
	cpuLimMilli := p99c * 1.1
	p90m, _ := qBytes(memSk, 0.90)
	p99m, _ := qBytes(memSk, 0.99)
	memReqBytes := p90m
	memLimBytes := p99m * 1.2
	rationale := fmt.Sprintf("resilience: P90 req, P99*1.1/1.2 limits, mode=%s", "resilience")
	return finalizeRec(in, cpuReqMilli, cpuLimMilli, memReqBytes, memLimBytes, rationale), nil
}

type burstStrategy struct{}

func (burstStrategy) Compute(in Input) (autosizev1.Recommendation, error) {
	cpuSk := effectiveCPUSketch(in)
	memSk := effectiveMemSketch(in)
	p40c, _ := qMilli(cpuSk, 0.40)
	p99c, _ := qMilli(cpuSk, 0.99)
	cpuReqMilli := p40c
	peakCPU := math.Max(p99c, in.CPUEShort)
	if in.ForecastCPU > 0 {
		peakCPU = math.Max(peakCPU, in.ForecastCPU)
	}
	cpuLimMilli := peakCPU
	p40m, _ := qBytes(memSk, 0.40)
	p99m, _ := qBytes(memSk, 0.99)
	memReqBytes := p40m
	peakMem := math.Max(p99m, memShortForBurst(in))
	memLimBytes := peakMem
	rationale := fmt.Sprintf("burst: P40 req, peak=max(P99,EMA_short), mode=%s", "burst")
	return finalizeRec(in, cpuReqMilli, cpuLimMilli, memReqBytes, memLimBytes, rationale), nil
}

func finalizeRec(in Input, cpuReqMilli, cpuLimMilli, memReqBytes, memLimBytes float64, rationale string) autosizev1.Recommendation {
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
	}
}
