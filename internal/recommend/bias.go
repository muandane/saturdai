package recommend

import autosizev1 "github.com/muandane/saturdai/api/v1"

// BiasProvider adjusts a recommendation based on learned feedback.
type BiasProvider interface {
	Apply(rec autosizev1.Recommendation, in Input) autosizev1.Recommendation
	// PredictionRatios returns clamped CPU and memory correction factors for rationale logging (1,1 means no bias).
	PredictionRatios(container string) (cpuRatio, memRatio float64)
}

// NoopBias applies no adjustment. Callers can use it instead of nil checks.
type NoopBias struct{}

func (NoopBias) Apply(rec autosizev1.Recommendation, _ Input) autosizev1.Recommendation {
	return rec
}

func (NoopBias) PredictionRatios(_ string) (float64, float64) { return 1.0, 1.0 }
