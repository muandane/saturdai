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
	cpuLimMilli, memLimBytes, k, cpuPred, memPred := mergeLimitsWithEMAPrediction(cpuLimMilli, memLimBytes, in, "cost")
	rationale := fmt.Sprintf(
		"cost: P50/P90 cpu & mem; limits=max(quantile,EMA_long+k*(short-long)) k=%.1f cpu_pred=%.0fm mem_pred=%.0f; mode=cost",
		k, cpuPred, memPred,
	)
	return finalizeRec(in, cpuReqMilli, cpuLimMilli, memReqBytes, memLimBytes, rationale), nil
}

type balancedStrategy struct {
	fallback bool // true when original mode was unknown (maps to balanced behavior)
}

func applySchedulerPressure(cpuReqMilli, memReqBytes float64, in Input) (float64, float64, string) {
	if in.SchedulerBalanceScore >= 0 && in.SchedulerBalanceScore < 0.15 {
		return cpuReqMilli * 1.15, memReqBytes * 1.15,
			fmt.Sprintf("; scheduler_pressure: high (balance=%.2f) request_inflated", in.SchedulerBalanceScore)
	}
	return cpuReqMilli, memReqBytes, ""
}

func (b balancedStrategy) Compute(in Input) (autosizev1.Recommendation, error) {
	mode := normalizeMode(in.Mode)
	cpuReqMilli, _ := qMilli(effectiveCPUSketch(in), 0.70)
	cpuLimMilli, _ := qMilli(effectiveCPUSketch(in), 0.95)
	memReqBytes, _ := qBytes(effectiveMemSketch(in), 0.70)
	memLimBytes, _ := qBytes(effectiveMemSketch(in), 0.95)
	cpuReqMilli, memReqBytes, schedNote := applySchedulerPressure(cpuReqMilli, memReqBytes, in)
	cpuLimMilli, memLimBytes, k, cpuPred, memPred := mergeLimitsWithEMAPrediction(cpuLimMilli, memLimBytes, in, "balanced")
	var rationale string
	if b.fallback {
		rationale = fmt.Sprintf(
			"balanced(default): P70/P95; limits=max(quantile,EMA_long+k*(short-long)) k=%.1f cpu_pred=%.0fm mem_pred=%.0f; mode=%s%s",
			k, cpuPred, memPred, mode, schedNote,
		)
	} else {
		rationale = fmt.Sprintf(
			"balanced: P70/P95 cpu & mem; limits=max(quantile,EMA_long+k*(short-long)) k=%.1f cpu_pred=%.0fm mem_pred=%.0f; mode=%s%s",
			k, cpuPred, memPred, mode, schedNote,
		)
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
	cpuReqMilli, memReqBytes, schedNote := applySchedulerPressure(cpuReqMilli, memReqBytes, in)
	if in.ForecastCPU > 0 {
		cpuLimMilli = math.Max(cpuLimMilli, in.ForecastCPU)
	}
	if in.ForecastMem > 0 {
		memLimBytes = math.Max(memLimBytes, in.ForecastMem)
	}
	cpuLimMilli, memLimBytes, k, cpuPred, memPred := mergeLimitsWithEMAPrediction(cpuLimMilli, memLimBytes, in, "resilience")
	rationale := fmt.Sprintf(
		"resilience: P90 req, P99*1.1/1.2 limits; limits=max(quantile,EMA_long+k*(short-long)) k=%.1f cpu_pred=%.0fm mem_pred=%.0f; mode=resilience%s",
		k, cpuPred, memPred, schedNote,
	)
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
	cpuLimMilli, memLimBytes, k, cpuPred, memPred := mergeLimitsWithEMAPrediction(cpuLimMilli, memLimBytes, in, "burst")
	rationale := fmt.Sprintf(
		"burst: P40 req, peak=max(P99,EMA_short/forecast); limits=max(peak,EMA_long+k*(short-long)) k=%.1f cpu_pred=%.0fm mem_pred=%.0f; mode=burst",
		k, cpuPred, memPred,
	)
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
