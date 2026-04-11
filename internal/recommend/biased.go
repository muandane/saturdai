package recommend

import (
	"fmt"
	"math"

	autosizev1 "github.com/muandane/saturdai/api/v1"
)

const biasRationaleEpsilon = 1e-6

type biasedEngine struct {
	base Engine
	bias BiasProvider
}

func (e biasedEngine) Compute(in Input) (autosizev1.Recommendation, error) {
	rec, err := e.base.Compute(in)
	if err != nil {
		return rec, err
	}
	rec = e.bias.Apply(rec, in)
	cpuR, memR := e.bias.PredictionRatios(in.ContainerName)
	if math.Abs(cpuR-1.0) > biasRationaleEpsilon || math.Abs(memR-1.0) > biasRationaleEpsilon {
		rec.Rationale += fmt.Sprintf("; bias: cpuRatio=%.2f memRatio=%.2f", cpuR, memR)
	}
	return rec, nil
}
