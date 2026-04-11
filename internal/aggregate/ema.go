package aggregate

const (
	// EMAShortAlpha reacts quickly to recent samples.
	EMAShortAlpha = 0.2
	// EMALongAlpha smooths noise.
	EMALongAlpha = 0.05
)

// UpdateEMA applies exponential moving average updates.
func UpdateEMA(prevShort, prevLong, sample float64) (short, long float64) {
	short = EMAShortAlpha*sample + (1-EMAShortAlpha)*prevShort
	long = EMALongAlpha*sample + (1-EMALongAlpha)*prevLong
	return short, long
}
