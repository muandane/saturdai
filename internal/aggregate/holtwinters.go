package aggregate

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
	if hour < 0 {
		hour = 0
	}
	if hour > 23 {
		hour %= 24
	}
	if h.Samples < 24 {
		h.Level = x
		h.Samples++
		return x
	}
	s := h.Season[hour]
	if s == 0 {
		s = 1.0
	}
	prevLevel := h.Level
	h.Level = h.Alpha*(x/s) + (1-h.Alpha)*(h.Level+h.Trend)
	h.Trend = h.Beta*(h.Level-prevLevel) + (1-h.Beta)*h.Trend
	h.Season[hour] = h.Gamma*(x/h.Level) + (1-h.Gamma)*s
	h.Samples++

	next := (hour + 1) % 24
	sn := h.Season[next]
	if sn == 0 {
		sn = 1.0
	}
	return (h.Level + h.Trend) * sn
}
