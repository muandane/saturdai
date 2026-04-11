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

// ContainerFeedback tracks prediction accuracy for one container (combined CPU/memory ratio EWMA).
type ContainerFeedback struct {
	PredictionRatio float64 `json:"predictionRatio"`
	SampleCount     int32   `json:"sampleCount"`
}

// RecordUsage compares observed kubelet usage to the previous reconcile's post-safety
// WorkloadProfile.Status.Recommendations (same basis as actuation), in millicores and bytes.
func (f *ContainerFeedback) RecordUsage(cpuActual, cpuRecommendedMilli, memActual, memRecommendedBytes float64) {
	var ratios []float64
	if cpuRecommendedMilli > 0 {
		ratios = append(ratios, cpuActual/cpuRecommendedMilli)
	}
	if memRecommendedBytes > 0 {
		ratios = append(ratios, memActual/memRecommendedBytes)
	}
	if len(ratios) == 0 {
		return
	}
	sum := 0.0
	for _, r := range ratios {
		sum += r
	}
	ratio := sum / float64(len(ratios))
	if f.SampleCount == 0 {
		f.PredictionRatio = ratio
	} else {
		f.PredictionRatio = biasAlpha*ratio + (1-biasAlpha)*f.PredictionRatio
	}
	f.SampleCount++
}

// LiveBias implements BiasProvider from persisted feedback state.
type LiveBias struct {
	state map[string]*ContainerFeedback
}

// NewLiveBias wraps feedback keyed by container name.
func NewLiveBias(state map[string]*ContainerFeedback) *LiveBias {
	return &LiveBias{state: state}
}

// PredictionRatio implements BiasProvider.
func (b *LiveBias) PredictionRatio(container string) float64 {
	if b == nil || b.state == nil {
		return 1.0
	}
	if f, ok := b.state[container]; ok && f != nil && f.SampleCount >= minSamples {
		return f.PredictionRatio
	}
	return 1.0
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
	ratio := clampFloat(f.PredictionRatio, biasMaxDown, biasMaxUp)
	if math.Abs(ratio-1.0) < biasRationaleEpsilon {
		return rec
	}
	rec.CPURequest = scaleMilliQty(rec.CPURequest, ratio)
	rec.CPULimit = scaleMilliQty(rec.CPULimit, ratio)
	rec.MemoryRequest = scaleBytesQty(rec.MemoryRequest, ratio)
	rec.MemoryLimit = scaleBytesQty(rec.MemoryLimit, ratio)
	return rec
}

func clampFloat(x, lo, hi float64) float64 {
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
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
