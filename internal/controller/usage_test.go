package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/muandane/saturdai/internal/kubelet"
)

func TestCpuHeteroScore(t *testing.T) {
	if v := cpuHeteroScore(map[string]nodeUsageSample{"a": {CPUMilli: 100}}); v != 0 {
		t.Fatalf("expected 0 for single node, got %v", v)
	}
	m := map[string]nodeUsageSample{
		"a": {CPUMilli: 100},
		"b": {CPUMilli: 200},
	}
	v := cpuHeteroScore(m)
	if v <= 0 || v > 1 {
		t.Fatalf("expected (0,1], got %v", v)
	}
}

func TestCollectUsageBreakdown_weightedMeanTwoPodsOneNode(t *testing.T) {
	ns := "default"
	cpu1 := uint64(100 * 1e6)
	cpu2 := uint64(300 * 1e6)
	mem1 := uint64(1000)
	mem2 := uint64(3000)
	summaries := map[string]*kubelet.Summary{
		"node-a": {
			Pods: []kubelet.PodStats{
				{
					PodRef: kubelet.PodReference{Namespace: ns, Name: "p1"},
					Containers: []kubelet.ContainerStats{
						{Name: "app", CPU: &kubelet.CPUStats{UsageNanoCores: &cpu1}, Memory: &kubelet.MemoryStats{WorkingSetBytes: &mem1}},
					},
				},
				{
					PodRef: kubelet.PodReference{Namespace: ns, Name: "p2"},
					Containers: []kubelet.ContainerStats{
						{Name: "app", CPU: &kubelet.CPUStats{UsageNanoCores: &cpu2}, Memory: &kubelet.MemoryStats{WorkingSetBytes: &mem2}},
					},
				},
			},
		},
	}
	pods := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: ns}, Spec: corev1.PodSpec{NodeName: "node-a"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "p2", Namespace: ns}, Spec: corev1.PodSpec{NodeName: "node-a"}},
	}
	perNode, cpuMean, memMean, _, _ := collectUsageBreakdown(summaries, ns, pods, "app")
	if len(perNode) != 1 {
		t.Fatalf("perNode: %v", perNode)
	}
	// Per-node mean: (100+300)/2 = 200m; workload mean over pods: (100+300)/2 = 200m
	if cpuMean < 199 || cpuMean > 201 {
		t.Fatalf("cpu mean want ~200 got %v", cpuMean)
	}
	if memMean < 1999 || memMean > 2001 {
		t.Fatalf("mem mean want ~2000 got %v", memMean)
	}
}
