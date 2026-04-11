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
	if r := e.bias.PredictionRatio(in.ContainerName); math.Abs(r-1.0) > biasRationaleEpsilon {
		rec.Rationale += fmt.Sprintf("; bias: predRatio=%.2f", r)
	}
	return rec, nil
}
