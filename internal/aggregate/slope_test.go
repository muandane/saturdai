package aggregate

import "testing"

func TestUpdateMemorySlope(t *testing.T) {
	t.Parallel()

	t.Run("cold_start_no_prior", func(t *testing.T) {
		t.Parallel()
		st, pos := UpdateMemorySlope(0, 100, 4, 5, false)
		if st != 0 || pos {
			t.Fatalf("got streak=%d slopePositive=%v want 0 false", st, pos)
		}
	})

	t.Run("increase_increments", func(t *testing.T) {
		t.Parallel()
		st, pos := UpdateMemorySlope(10, 20, 0, 5, true)
		if st != 1 || pos {
			t.Fatalf("got streak=%d slopePositive=%v want 1 false", st, pos)
		}
	})

	t.Run("flat_resets", func(t *testing.T) {
		t.Parallel()
		st, pos := UpdateMemorySlope(20, 20, 4, 5, true)
		if st != 0 || pos {
			t.Fatalf("got streak=%d slopePositive=%v want 0 false", st, pos)
		}
	})

	t.Run("decrease_resets", func(t *testing.T) {
		t.Parallel()
		st, pos := UpdateMemorySlope(30, 10, 4, 5, true)
		if st != 0 || pos {
			t.Fatalf("got streak=%d slopePositive=%v want 0 false", st, pos)
		}
	})

	t.Run("fifth_increase_sets_positive", func(t *testing.T) {
		t.Parallel()
		st, pos := UpdateMemorySlope(40, 50, 4, 5, true)
		if st != 5 || !pos {
			t.Fatalf("got streak=%d slopePositive=%v want 5 true", st, pos)
		}
	})

	t.Run("threshold_default_negative", func(t *testing.T) {
		t.Parallel()
		st, pos := UpdateMemorySlope(1, 2, 4, 0, true)
		if st != 5 || !pos {
			t.Fatalf("threshold 0 should use default 5: got streak=%d pos=%v", st, pos)
		}
	})
}
