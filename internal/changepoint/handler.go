package changepoint

import "time"

// ShiftEvent is emitted when CUSUM detects a distributional shift.
type ShiftEvent struct {
	ContainerName string
	Resource      string // "cpu" or "memory"
	At            time.Time
	OldMean       float64
	NewMean       float64
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

// SketchDecayHandler forwards shifts to a reconciler callback (sketch reset owned by caller).
type SketchDecayHandler struct {
	OnDecay func(container, resource string)
}

// OnShift implements Handler.
func (h SketchDecayHandler) OnShift(e ShiftEvent) {
	if h.OnDecay != nil {
		h.OnDecay(e.ContainerName, e.Resource)
	}
}
