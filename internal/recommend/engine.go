package recommend

import autosizev1 "github.com/muandane/saturdai/api/v1"

// Engine computes a single container recommendation.
type Engine interface {
	Compute(in Input) (autosizev1.Recommendation, error)
}

// New returns the Engine for mode wrapped with bias adjustment.
func New(mode string, bias BiasProvider) Engine {
	if bias == nil {
		bias = NoopBias{}
	}
	strategies := map[string]Engine{
		"cost":       costStrategy{},
		"balanced":   balancedStrategy{},
		"resilience": resilienceStrategy{},
		"burst":      burstStrategy{},
	}
	norm := normalizeMode(mode)
	base, ok := strategies[norm]
	if !ok {
		base = balancedStrategy{fallback: true}
	}
	return biasedEngine{base: base, bias: bias}
}
