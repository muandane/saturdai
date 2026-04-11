package aggregate

import "math"

// FiniteOrZero returns x if it is a finite number; otherwise 0 (NaN and ±Inf become 0).
func FiniteOrZero(x float64) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return 0
	}
	return x
}

// IsFinite reports whether x is neither NaN nor ±Inf.
func IsFinite(x float64) bool {
	return !math.IsNaN(x) && !math.IsInf(x, 0)
}
