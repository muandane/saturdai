package controller

// isRestartSpike reports whether restart count jumped by more than 3 since the last persisted status (requires baseline).
func isRestartSpike(prevRestart, currentMax int32, baselineSeen bool) bool {
	if !baselineSeen {
		return false
	}
	return int64(currentMax)-int64(prevRestart) > 3
}

// restartPauseAfterReconcile returns the new status.downsizePauseCyclesRemaining value.
// When a spike is observed (baselineSeen && anySpike), the counter is set to 2; otherwise it decrements if positive.
func restartPauseAfterReconcile(baselineSeen, anySpike bool, pauseRemaining int32) int32 {
	if baselineSeen && anySpike {
		return 2
	}
	if pauseRemaining > 0 {
		return pauseRemaining - 1
	}
	return 0
}
