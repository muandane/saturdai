package changepoint

import "math"

// State holds per-resource CUSUM accumulators (stored in MLState, persisted).
type State struct {
	SPos float64 `json:"sPos"`
	SNeg float64 `json:"sNeg"`
}

// Config tunes sensitivity.
type Config struct {
	K float64 // allowance
	H float64 // decision threshold
}

// DefaultCPUConfig targets millicore-scale shifts.
var DefaultCPUConfig = Config{K: 25, H: 125}

// DefaultMemConfig targets byte-scale shifts (~32MiB K, ~80MiB H).
var DefaultMemConfig = Config{K: 16 * 1024 * 1024, H: 80 * 1024 * 1024}

// Update advances CUSUM and returns true when a shift is detected.
// target is the reference mean (pre-sample EMALong).
func (s *State) Update(x, target float64, cfg Config) bool {
	s.SPos = math.Max(0, s.SPos+x-target-cfg.K)
	s.SNeg = math.Max(0, s.SNeg-x+target-cfg.K)
	return s.SPos > cfg.H || s.SNeg > cfg.H
}

// Reset clears accumulators after a confirmed shift.
func (s *State) Reset() {
	s.SPos, s.SNeg = 0, 0
}
