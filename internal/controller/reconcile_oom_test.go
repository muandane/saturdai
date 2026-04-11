package controller

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	autosizev1 "github.com/muandane/saturdai/api/v1"
)

func TestApplyLastOOMKillFromSnapshot_nilClears(t *testing.T) {
	st := autosizev1.ProfileContainerStatus{
		Stats: autosizev1.ContainerResourceStats{
			LastOOMKill: func() *metav1.Time {
				t0 := metav1.NewTime(time.Unix(100, 0))
				return &t0
			}(),
		},
	}
	applyLastOOMKillFromSnapshot(&st, nil)
	if st.Stats.LastOOMKill != nil {
		t.Fatalf("expected LastOOMKill cleared, got %v", st.Stats.LastOOMKill)
	}
}

func TestApplyLastOOMKillFromSnapshot_copiesTime(t *testing.T) {
	src := metav1.NewTime(time.Date(2026, 4, 2, 8, 0, 0, 0, time.UTC))
	st := autosizev1.ProfileContainerStatus{}
	applyLastOOMKillFromSnapshot(&st, &src)

	if st.Stats.LastOOMKill == nil {
		t.Fatal("expected LastOOMKill set")
	}
	if !st.Stats.LastOOMKill.Equal(&src) {
		t.Fatalf("got %v want %v", st.Stats.LastOOMKill.Time, src.Time)
	}
	if st.Stats.LastOOMKill == &src {
		t.Fatal("expected copy, not same pointer as source")
	}

	want := src.Time
	src = metav1.NewTime(time.Unix(0, 0))
	if !st.Stats.LastOOMKill.Time.Equal(want) {
		t.Fatalf("reassigning source variable changed status: got %v want %v", st.Stats.LastOOMKill.Time, want)
	}
}
