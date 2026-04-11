package recommend

import (
	"math"

	"k8s.io/apimachinery/pkg/api/resource"

	autosizev1 "github.com/muandane/saturdai/api/v1"
)

const (
	biasAlpha   = 0.1
	minSamples  = 10
	biasMaxUp   = 1.30
	biasMaxDown = 0.85
)

// ContainerFeedback tracks prediction accuracy per resource (EWMA of actual/recommended).
type ContainerFeedback struct {
	CPURatio    float64 `json:"cpuRatio"`
	MemRatio    float64 `json:"memRatio"`
	CPUSamples  int32   `json:"cpuSamples,omitempty"`
	MemSamples  int32   `json:"memSamples,omitempty"`
	SampleCount int32   `json:"sampleCount"`
	// PredictionRatioLegacy is only present in older persisted JSON; migrated on load.
	PredictionRatioLegacy float64 `json:"predictionRatio,omitempty"`
}

// MigrateLegacyFeedback copies legacy predictionRatio into CPURatio/MemRatio when new fields are unset.
func MigrateLegacyFeedback(f *ContainerFeedback) {
	if f == nil {
		return
	}
	if f.PredictionRatioLegacy > 0 && f.CPURatio == 0 && f.MemRatio == 0 {
		f.CPURatio = f.PredictionRatioLegacy
		f.MemRatio = f.PredictionRatioLegacy
	}
	f.PredictionRatioLegacy = 0
	SanitizeFeedbackRatios(f)
}

// SanitizeFeedbackRatios clears non-finite ratio fields (e.g. corrupt persisted JSON) so mlstate marshals safely.
func SanitizeFeedbackRatios(f *ContainerFeedback) {
	if f == nil {
		return
	}
	if !ratioFieldOK(f.CPURatio) {
		f.CPURatio = 0
	}
	if !ratioFieldOK(f.MemRatio) {
		f.MemRatio = 0
	}
}

func ratioFieldOK(x float64) bool {
	return !math.IsNaN(x) && !math.IsInf(x, 0)
}

// RecordUsage compares observed kubelet usage to the previous reconcile's post-safety
// WorkloadProfile.Status.Recommendations (same basis as actuation), in millicores and bytes.
func (f *ContainerFeedback) RecordUsage(cpuActual, cpuRecommendedMilli, memActual, memRecommendedBytes float64) {
	if cpuRecommendedMilli > 0 && ratioFieldOK(cpuActual) && ratioFieldOK(cpuRecommendedMilli) {
		r := cpuActual / cpuRecommendedMilli
		if !ratioFieldOK(r) {
			r = 1.0
		}
		if f.CPUSamples == 0 {
			f.CPURatio = r
		} else {
			f.CPURatio = biasAlpha*r + (1-biasAlpha)*f.CPURatio
		}
		f.CPURatio = feedbackFiniteRatio(f.CPURatio)
		f.CPUSamples++
	}
	if memRecommendedBytes > 0 && ratioFieldOK(memActual) && ratioFieldOK(memRecommendedBytes) {
		r := memActual / memRecommendedBytes
		if !ratioFieldOK(r) {
			r = 1.0
		}
		if f.MemSamples == 0 {
			f.MemRatio = r
		} else {
			f.MemRatio = biasAlpha*r + (1-biasAlpha)*f.MemRatio
		}
		f.MemRatio = feedbackFiniteRatio(f.MemRatio)
		f.MemSamples++
	}
	f.SampleCount++
}

func feedbackFiniteRatio(x float64) float64 {
	if ratioFieldOK(x) {
		return x
	}
	return 1.0
}

// LiveBias implements BiasProvider from persisted feedback state.
type LiveBias struct {
	state map[string]*ContainerFeedback
}

// NewLiveBias wraps feedback keyed by container name.
func NewLiveBias(state map[string]*ContainerFeedback) *LiveBias {
	return &LiveBias{state: state}
}

// PredictionRatios implements BiasProvider (clamped factors for rationale; matches Apply scaling).
func (b *LiveBias) PredictionRatios(container string) (cpuRatio, memRatio float64) {
	if b == nil || b.state == nil {
		return 1.0, 1.0
	}
	f, ok := b.state[container]
	if !ok || f == nil || f.SampleCount < minSamples {
		return 1.0, 1.0
	}
	cpuR, memR := 1.0, 1.0
	if f.CPUSamples > 0 && f.CPURatio > 0 {
		cpuR = clampBiasRatio(f.CPURatio)
	}
	if f.MemSamples > 0 && f.MemRatio > 0 {
		memR = clampBiasRatio(f.MemRatio)
	}
	return cpuR, memR
}

// Apply implements BiasProvider.
func (b *LiveBias) Apply(rec autosizev1.Recommendation, in Input) autosizev1.Recommendation {
	if b == nil || b.state == nil {
		return rec
	}
	f, ok := b.state[in.ContainerName]
	if !ok || f == nil || f.SampleCount < minSamples {
		return rec
	}
	cpuR, memR := 1.0, 1.0
	if f.CPUSamples > 0 && f.CPURatio > 0 {
		cpuR = clampBiasRatio(f.CPURatio)
	}
	if f.MemSamples > 0 && f.MemRatio > 0 {
		memR = clampBiasRatio(f.MemRatio)
	}
	if math.Abs(cpuR-1.0) < biasRationaleEpsilon && math.Abs(memR-1.0) < biasRationaleEpsilon {
		return rec
	}
	rec.CPURequest = scaleMilliQty(rec.CPURequest, cpuR)
	rec.CPULimit = scaleMilliQty(rec.CPULimit, cpuR)
	rec.MemoryRequest = scaleBytesQty(rec.MemoryRequest, memR)
	rec.MemoryLimit = scaleBytesQty(rec.MemoryLimit, memR)
	return rec
}

func clampBiasRatio(x float64) float64 {
	if !ratioFieldOK(x) {
		return 1.0
	}
	if x < biasMaxDown {
		return biasMaxDown
	}
	if x > biasMaxUp {
		return biasMaxUp
	}
	return x
}

func scaleMilliQty(q resource.Quantity, ratio float64) resource.Quantity {
	v := float64(q.MilliValue()) * ratio
	return *resource.NewMilliQuantity(int64(math.Round(v)), resource.DecimalSI)
}

func scaleBytesQty(q resource.Quantity, ratio float64) resource.Quantity {
	v := float64(q.Value()) * ratio
	return *resource.NewQuantity(int64(math.Round(v)), resource.BinarySI)
}
