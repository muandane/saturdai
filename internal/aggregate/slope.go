package aggregate

// DefaultMemorySlopeCycles is N from spec §6: consecutive reconcile cycles of strictly increasing
// memory EMAShort before SlopePositive blocks downsizing.
const DefaultMemorySlopeCycles int32 = 5

// UpdateMemorySlope updates streak and slopePositive per spec §6: if newEMAShort > prevEMAShort then
// increment streak, else reset to 0. When streak >= threshold, slopePositive is true.
//
// priorObserved is false on cold start (no memory LastUpdated yet): streak stays 0 and slopePositive false
// so the first sample does not count as an "increase" from zero.
func UpdateMemorySlope(prevEMAShort, newEMAShort float64, streak int32, threshold int32, priorObserved bool) (newStreak int32, slopePositive bool) {
	if threshold <= 0 {
		threshold = DefaultMemorySlopeCycles
	}
	if !priorObserved {
		return 0, false
	}
	if newEMAShort > prevEMAShort {
		newStreak = streak + 1
	} else {
		newStreak = 0
	}
	slopePositive = newStreak >= threshold
	return newStreak, slopePositive
}
