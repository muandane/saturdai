package controller

import (
	"context"

	autosizev1 "github.com/muandane/saturdai/api/v1"
	"github.com/muandane/saturdai/internal/aggregate"
	"github.com/muandane/saturdai/internal/mlstate"
	"github.com/muandane/saturdai/internal/recommend"
)

func (r *WorkloadProfileReconciler) saveMLState(ctx context.Context, profile *autosizev1.WorkloadProfile, state *mlstate.MLState) error {
	if r.MLState == nil {
		return nil
	}
	return r.MLState.Save(ctx, profile, state)
}

func lastRecommendationFor(profile *autosizev1.WorkloadProfile, container string) *autosizev1.Recommendation {
	for i := range profile.Status.Recommendations {
		if profile.Status.Recommendations[i].ContainerName == container {
			return &profile.Status.Recommendations[i]
		}
	}
	return nil
}

func ensureContainerCUSUM(ml *mlstate.MLState, name string) *mlstate.ContainerCUSUM {
	if ml.CUSUM[name] == nil {
		ml.CUSUM[name] = &mlstate.ContainerCUSUM{}
	}
	return ml.CUSUM[name]
}

func ensureContainerFeedback(ml *mlstate.MLState, name string) *recommend.ContainerFeedback {
	if ml.Feedback[name] == nil {
		ml.Feedback[name] = &recommend.ContainerFeedback{}
	}
	return ml.Feedback[name]
}

func ensureContainerHW(ml *mlstate.MLState, name string) *mlstate.ContainerHW {
	if ml.HW[name] == nil {
		c := aggregate.DefaultHWState()
		m := aggregate.DefaultHWState()
		ml.HW[name] = &mlstate.ContainerHW{CPU: &c, Memory: &m}
	}
	if ml.HW[name].CPU == nil {
		c := aggregate.DefaultHWState()
		ml.HW[name].CPU = &c
	}
	if ml.HW[name].Memory == nil {
		m := aggregate.DefaultHWState()
		ml.HW[name].Memory = &m
	}
	return ml.HW[name]
}

func quadSketchGet(sk []string, i int) string {
	if i >= 0 && i < len(sk) {
		return sk[i]
	}
	return ""
}

func quadSketchSet(sk *[]string, i int, v string) {
	for len(*sk) <= i {
		*sk = append(*sk, "")
	}
	(*sk)[i] = v
}
