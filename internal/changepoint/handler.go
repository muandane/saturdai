package changepoint

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
)

// ShiftEvent is emitted when CUSUM detects a distributional shift.
type ShiftEvent struct {
	ContainerName string
	Resource      string // "cpu" or "memory"
	At            time.Time
	OldMean       float64
	NewMean       float64
	// InvolvedObject is the WorkloadProfile (or other) used as the Kubernetes Event subject.
	InvolvedObject runtime.Object
}

// Handler reacts to a detected shift.
type Handler interface {
	OnShift(e ShiftEvent)
}

// Detector notifies registered handlers of shift events.
type Detector struct {
	handlers []Handler
}

// NewDetector builds a detector with optional handlers.
func NewDetector(handlers ...Handler) *Detector {
	return &Detector{handlers: append([]Handler(nil), handlers...)}
}

// Notify invokes all handlers with e.
func (d *Detector) Notify(e ShiftEvent) {
	if d == nil {
		return
	}
	for _, h := range d.handlers {
		if h != nil {
			h.OnShift(e)
		}
	}
}

// EventRecorderHandler emits a Kubernetes Event on each shift (primary observability path).
type EventRecorderHandler struct {
	Recorder record.EventRecorder
}

// OnShift implements Handler.
func (h EventRecorderHandler) OnShift(e ShiftEvent) {
	if h.Recorder == nil || e.InvolvedObject == nil {
		return
	}
	h.Recorder.Eventf(e.InvolvedObject, corev1.EventTypeNormal, "CusumShift",
		"CUSUM shift detected for container %s (%s): oldMean=%.2f newMean=%.2f",
		e.ContainerName, e.Resource, e.OldMean, e.NewMean)
}
