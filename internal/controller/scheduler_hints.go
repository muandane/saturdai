package controller

import (
	"context"
	"math"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type nodeSchedulerState struct {
	AllocatableCPUMilli int64
	AllocatableMemBytes int64
	RequestedCPUMilli   int64
	RequestedMemBytes   int64
}

func podRequestsTotal(p *corev1.Pod) (cpuMilli, memBytes int64) {
	for i := range p.Spec.Containers {
		cpuMilli += p.Spec.Containers[i].Resources.Requests.Cpu().MilliValue()
		memBytes += p.Spec.Containers[i].Resources.Requests.Memory().Value()
	}
	for i := range p.Spec.InitContainers {
		cpuMilli += p.Spec.InitContainers[i].Resources.Requests.Cpu().MilliValue()
		memBytes += p.Spec.InitContainers[i].Resources.Requests.Memory().Value()
	}
	return cpuMilli, memBytes
}

// fetchNodeSchedulerStates returns allocatable vs requested for each node name (best-effort).
func fetchNodeSchedulerStates(ctx context.Context, log logr.Logger, c client.Client, nodeNames []string) map[string]nodeSchedulerState {
	result := make(map[string]nodeSchedulerState, len(nodeNames))
	for _, name := range nodeNames {
		if name == "" {
			continue
		}
		node := &corev1.Node{}
		if err := c.Get(ctx, client.ObjectKey{Name: name}, node); err != nil {
			log.V(1).Info("scheduler hints: skip node", "node", name, "reason", err.Error())
			continue
		}
		allCPU := node.Status.Allocatable.Cpu().MilliValue()
		allMem := node.Status.Allocatable.Memory().Value()

		podList := &corev1.PodList{}
		if err := c.List(ctx, podList, client.MatchingFields{"spec.nodeName": name}); err != nil {
			log.V(1).Info("scheduler hints: list pods for node failed", "node", name, "reason", err.Error())
			continue
		}
		var reqCPU, reqMem int64
		for i := range podList.Items {
			p := &podList.Items[i]
			if p.Status.Phase == corev1.PodSucceeded || p.Status.Phase == corev1.PodFailed {
				continue
			}
			cpu, mem := podRequestsTotal(p)
			reqCPU += cpu
			reqMem += mem
		}

		result[name] = nodeSchedulerState{
			AllocatableCPUMilli: allCPU,
			AllocatableMemBytes: allMem,
			RequestedCPUMilli:   reqCPU,
			RequestedMemBytes:   reqMem,
		}
	}
	return result
}

// leastAllocatedScore mirrors a kube-scheduler LeastAllocated–style cluster summary: mean of per-node
// (freeCPUfrac + freeMemfrac) / 2 in [0,1]. Higher means more spare capacity (balanced); lower means packed.
func leastAllocatedScore(states map[string]nodeSchedulerState) float64 {
	if len(states) == 0 {
		return 0
	}
	var total float64
	var count int
	for _, s := range states {
		if s.AllocatableCPUMilli == 0 || s.AllocatableMemBytes == 0 {
			continue
		}
		cpuFree := 1.0 - float64(s.RequestedCPUMilli)/float64(s.AllocatableCPUMilli)
		memFree := 1.0 - float64(s.RequestedMemBytes)/float64(s.AllocatableMemBytes)
		total += (math.Max(0, cpuFree) + math.Max(0, memFree)) / 2
		count++
	}
	if count == 0 {
		return 0
	}
	return total / float64(count)
}

func nodePressureLabel(score float64) string {
	switch {
	case score > 0.4:
		return "low"
	case score > 0.15:
		return "medium"
	default:
		return "high"
	}
}
