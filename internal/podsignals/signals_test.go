package podsignals

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func podWithOOM(containerName string, finishedAt metav1.Time) *corev1.Pod {
	return &corev1.Pod{
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         containerName,
					RestartCount: 1,
					LastTerminationState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Reason:     "OOMKilled",
							FinishedAt: finishedAt,
						},
					},
				},
			},
		},
	}
}

func TestMergePodStatus_twoPodsKeepsMostRecentOOMFinishedAt(t *testing.T) {
	t1 := metav1.NewTime(time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC))
	t2 := metav1.NewTime(time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC))

	s := NewSnapshot()
	// Later OOM first, then earlier — result must still be the later timestamp.
	s.MergePodStatus(podWithOOM("app", t2))
	s.MergePodStatus(podWithOOM("app", t1))

	got := s.LastOOMKill["app"]
	if got == nil {
		t.Fatal("expected lastOOMKill for app")
	}
	if !got.Equal(&t2) {
		t.Fatalf("got %v want %v (most recent finishedAt)", got.Time, t2.Time)
	}
}

func TestMergePodStatus_twoPodsOrderIndependent(t *testing.T) {
	t1 := metav1.NewTime(time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC))
	t2 := metav1.NewTime(time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC))

	s := NewSnapshot()
	s.MergePodStatus(podWithOOM("app", t1))
	s.MergePodStatus(podWithOOM("app", t2))

	got := s.LastOOMKill["app"]
	if got == nil {
		t.Fatal("expected lastOOMKill for app")
	}
	if !got.Equal(&t2) {
		t.Fatalf("got %v want %v", got.Time, t2.Time)
	}
}

func TestMergePodStatus_singlePodOOM(t *testing.T) {
	t0 := metav1.NewTime(time.Date(2026, 4, 1, 15, 30, 0, 0, time.UTC))
	s := NewSnapshot()
	s.MergePodStatus(podWithOOM("worker", t0))

	got := s.LastOOMKill["worker"]
	if got == nil || !got.Equal(&t0) {
		t.Fatalf("got %v want %v", got, t0)
	}
}

func TestMergePodStatus_noOOMLeavesLastOOMKillEmpty(t *testing.T) {
	s := NewSnapshot()
	s.MergePodStatus(&corev1.Pod{
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "app", RestartCount: 0},
			},
		},
	})
	if len(s.LastOOMKill) != 0 {
		t.Fatalf("LastOOMKill: got %d entries want 0", len(s.LastOOMKill))
	}
}

func TestMergePodStatus_nonOOMTerminationIgnoredForLastOOMKill(t *testing.T) {
	finished := metav1.NewTime(time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC))
	s := NewSnapshot()
	s.MergePodStatus(&corev1.Pod{
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "app",
					LastTerminationState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Reason:     "Error",
							FinishedAt: finished,
						},
					},
				},
			},
		},
	})
	if _, ok := s.LastOOMKill["app"]; ok {
		t.Fatal("non-OOM termination must not set LastOOMKill")
	}
}

func TestMergePodStatus_nilPodNoOp(t *testing.T) {
	s := NewSnapshot()
	s.MergePodStatus(nil)
	if len(s.LastOOMKill) != 0 {
		t.Fatal("nil pod should not mutate snapshot")
	}
}
