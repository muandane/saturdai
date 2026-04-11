package mlstate

import (
	"github.com/muandane/saturdai/internal/aggregate"
	"github.com/muandane/saturdai/internal/changepoint"
	"github.com/muandane/saturdai/internal/recommend"
)

// MLState holds learned controller state for one WorkloadProfile (ConfigMap JSON, not CR status).
type MLState struct {
	CUSUM    map[string]*ContainerCUSUM              `json:"cusum,omitempty"`
	Feedback map[string]*recommend.ContainerFeedback `json:"feedback,omitempty"`
	HW       map[string]*ContainerHW                 `json:"hw,omitempty"`
}

// ContainerCUSUM is CUSUM state per resource for one container.
type ContainerCUSUM struct {
	CPU    changepoint.State `json:"cpu"`
	Memory changepoint.State `json:"memory"`
}

// ContainerHW holds Holt-Winters state per resource.
type ContainerHW struct {
	CPU    *aggregate.HWState `json:"cpu,omitempty"`
	Memory *aggregate.HWState `json:"memory,omitempty"`
}

// New returns an empty MLState with initialized maps.
func New() *MLState {
	return &MLState{
		CUSUM:    map[string]*ContainerCUSUM{},
		Feedback: map[string]*recommend.ContainerFeedback{},
		HW:       map[string]*ContainerHW{},
	}
}
