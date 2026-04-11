package controller

import (
	"testing"
	"time"
)

func TestUtcQuadrantIndex(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		tm   time.Time
		want int
	}{
		{"midnight UTC", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), 0},
		{"05:59 UTC", time.Date(2026, 1, 1, 5, 59, 0, 0, time.UTC), 0},
		{"06:00 UTC", time.Date(2026, 1, 1, 6, 0, 0, 0, time.UTC), 1},
		{"11:59 UTC", time.Date(2026, 1, 1, 11, 59, 0, 0, time.UTC), 1},
		{"12:00 UTC", time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC), 2},
		{"17:59 UTC", time.Date(2026, 1, 1, 17, 59, 0, 0, time.UTC), 2},
		{"18:00 UTC", time.Date(2026, 1, 1, 18, 0, 0, 0, time.UTC), 3},
		{"23:59 UTC", time.Date(2026, 1, 1, 23, 59, 0, 0, time.UTC), 3},
		{"EST 02:00 wall is 07:00 UTC quad 1", time.Date(2026, 1, 1, 2, 0, 0, 0, time.FixedZone("EST", -5*3600)), 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := utcQuadrantIndex(tc.tm); got != tc.want {
				t.Fatalf("utcQuadrantIndex(%v) = %d, want %d", tc.tm, got, tc.want)
			}
		})
	}
}
