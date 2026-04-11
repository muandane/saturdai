package recommend

import "math"

// kForMode returns spec §8 k for the normalized profile mode.
func kForMode(mode string) float64 {
	switch normalizeMode(mode) {
	case "cost":
		return 0.5
	case "balanced":
		return 1.0
	case "resilience":
		return 1.5
	case "burst":
		return 2.0
	default:
		return 1.0
	}
}

// emaPrediction returns EMA_long + k*(EMA_short - EMA_long) (spec §7).
func emaPrediction(emaShort, emaLong, k float64) float64 {
	return emaLong + k*(emaShort-emaLong)
}

// limitWithPrediction returns max(quantileLimit, emaPrediction) per LLD-060 (limits only).
func limitWithPrediction(quantileLimit, emaShort, emaLong, k float64) float64 {
	return math.Max(quantileLimit, emaPrediction(emaShort, emaLong, k))
}

// mergeLimitsWithEMAPrediction applies spec §7–8 to CPU/memory limits; returns updated limits, k, and raw predictions (for rationale).
func mergeLimitsWithEMAPrediction(cpuLimMilli, memLimBytes float64, in Input, mode string) (cpuOut, memOut, k, cpuPred, memPred float64) {
	k = kForMode(mode)
	cpuOut = limitWithPrediction(cpuLimMilli, in.CPUEShort, in.CPUELong, k)
	memOut = limitWithPrediction(memLimBytes, in.MemShort, in.MemLong, k)
	cpuPred = emaPrediction(in.CPUEShort, in.CPUELong, k)
	memPred = emaPrediction(in.MemShort, in.MemLong, k)
	return cpuOut, memOut, k, cpuPred, memPred
}
