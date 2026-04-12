package controller

import (
	"math"

	corev1 "k8s.io/api/core/v1"

	"github.com/muandane/saturdai/internal/aggregate"
	"github.com/muandane/saturdai/internal/kubelet"
)

// nodeUsageSample is mean CPU (millicores) and memory (bytes) for one container on one node.
type nodeUsageSample struct {
	CPUMilli float64
	MemBytes float64
}

// collectUsageBreakdown aggregates kubelet stats per node and workload-level means weighted by pod count.
func collectUsageBreakdown(summaries map[string]*kubelet.Summary, ns string, pods []corev1.Pod, container string) (
	perNode map[string]nodeUsageSample,
	podCounts map[string]int,
	cpuMilliMean float64,
	memBytesMean float64,
	throttled uint64,
	usage uint64,
) {
	type acc struct {
		cpuSum, memSum float64
		cpuN, memN     int
	}
	byNode := map[string]*acc{}
	podCounts = map[string]int{}

	for _, pod := range pods {
		if pod.Namespace != ns || pod.Spec.NodeName == "" {
			continue
		}
		sum := summaries[pod.Spec.NodeName]
		if sum == nil {
			continue
		}
		for _, ps := range sum.Pods {
			if ps.PodRef.Namespace != pod.Namespace || ps.PodRef.Name != pod.Name {
				continue
			}
			for _, cs := range ps.Containers {
				if cs.Name != container {
					continue
				}
				node := pod.Spec.NodeName
				a := byNode[node]
				if a == nil {
					a = &acc{}
					byNode[node] = a
				}
				if cs.CPU != nil && cs.CPU.UsageNanoCores != nil {
					a.cpuSum += float64(*cs.CPU.UsageNanoCores) / 1e6
					a.cpuN++
					if cs.CPU.ThrottledUsageNanoCores != nil {
						throttled += *cs.CPU.ThrottledUsageNanoCores
					}
					if cs.CPU.UsageNanoCores != nil {
						usage += *cs.CPU.UsageNanoCores
					}
				}
				if cs.Memory != nil && cs.Memory.WorkingSetBytes != nil {
					a.memSum += float64(*cs.Memory.WorkingSetBytes)
					a.memN++
				}
				podCounts[node]++
				break
			}
		}
	}

	perNode = make(map[string]nodeUsageSample, len(byNode))
	var cpuWeighted, memWeighted float64
	var totalPods int
	for node, a := range byNode {
		var s nodeUsageSample
		if a.cpuN > 0 {
			s.CPUMilli = a.cpuSum / float64(a.cpuN)
		}
		if a.memN > 0 {
			s.MemBytes = a.memSum / float64(a.memN)
		}
		perNode[node] = s
		c := podCounts[node]
		if c > 0 {
			cpuWeighted += s.CPUMilli * float64(c)
			memWeighted += s.MemBytes * float64(c)
			totalPods += c
		}
	}
	if totalPods > 0 {
		cpuMilliMean = cpuWeighted / float64(totalPods)
		memBytesMean = memWeighted / float64(totalPods)
	}
	return perNode, podCounts,
		aggregate.FiniteOrZero(cpuMilliMean),
		aggregate.FiniteOrZero(memBytesMean),
		throttled, usage
}

// cpuHeteroScore returns (max-min)/max(mean,1e-6) capped to [0,1] when len>=2; else 0.
func cpuHeteroScore(perNode map[string]nodeUsageSample) float64 {
	if len(perNode) < 2 {
		return 0
	}
	minV := math.MaxFloat64
	maxV := 0.0
	var sum float64
	for _, v := range perNode {
		c := v.CPUMilli
		sum += c
		if c < minV {
			minV = c
		}
		if c > maxV {
			maxV = c
		}
	}
	mean := sum / float64(len(perNode))
	den := math.Max(mean, 1e-6)
	spread := (maxV - minV) / den
	if spread < 0 {
		return 0
	}
	if spread > 1 {
		return 1
	}
	return spread
}
