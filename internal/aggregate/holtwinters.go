package aggregate

import "math"

// hwLevelEpsilon avoids division by zero in the seasonal ratio x/Level.
const hwLevelEpsilon = 1e-12

// HWState is Holt-Winters triple exponential smoothing state (hourly seasonality).
type HWState struct {
	Level   float64     `json:"level"`
	Trend   float64     `json:"trend"`
	Season  [24]float64 `json:"season"`
	Alpha   float64     `json:"alpha"`
	Beta    float64     `json:"beta"`
	Gamma   float64     `json:"gamma"`
	Samples int32       `json:"samples"`
}

// DefaultHWState returns defaults (season indices 1.0).
func DefaultHWState() HWState {
	var s HWState
	s.Alpha, s.Beta, s.Gamma = 0.3, 0.1, 0.1
	for i := range s.Season {
		s.Season[i] = 1.0
	}
	return s
}

// Update advances state with sample x at hour (0–23). Returns forecast for the next hour.
func (h *HWState) Update(x float64, hour int) float64 {
	x = FiniteOrZero(x)
	if hour < 0 {
		hour = 0
	}
	if hour > 23 {
		hour %= 24
	}
	if h.Samples < 24 {
		h.Level += (x - h.Level) / float64(h.Samples+1)
		h.Samples++
		if !IsFinite(h.Level) {
			h.Level = 0
		}
		return x
	}
	s := h.Season[hour]
	if s == 0 || !IsFinite(s) {
		s = 1.0
	}
	prevLevel := h.Level
	h.Level = h.Alpha*(x/s) + (1-h.Alpha)*(h.Level+h.Trend)
	h.Trend = h.Beta*(h.Level-prevLevel) + (1-h.Beta)*h.Trend
	// Season uses x/Level; Level==0 yields NaN/Inf and breaks JSON persistence.
	levelForRatio := h.Level
	if math.Abs(levelForRatio) < hwLevelEpsilon {
		levelForRatio = hwLevelEpsilon
	}
	ratio := x / levelForRatio
	if !IsFinite(ratio) {
		ratio = 1.0
	}
	h.Season[hour] = h.Gamma*ratio + (1-h.Gamma)*s
	if !IsFinite(h.Season[hour]) {
		h.Season[hour] = s
	}
	h.Samples++

	h.sanitizeState()
	next := (hour + 1) % 24
	sn := h.Season[next]
	if sn == 0 || !IsFinite(sn) {
		sn = 1.0
	}
	out := (h.Level + h.Trend) * sn
	if !IsFinite(out) {
		return 0
	}
	return out
}

func (h *HWState) sanitizeState() {
	if !IsFinite(h.Level) {
		h.Level = 0
	}
	if !IsFinite(h.Trend) {
		h.Trend = 0
	}
	for i := range h.Season {
		if !IsFinite(h.Season[i]) {
			h.Season[i] = 1.0
		}
	}
}

// Sanitize replaces non-finite Level, Trend, and Season entries so persisted JSON remains valid.
func (h *HWState) Sanitize() {
	if h == nil {
		return
	}
	h.sanitizeState()
}
