package controller

import (
	"sort"
	"time"

	"github.com/go-logr/logr"

	autosizev1 "github.com/muandane/saturdai/api/v1"
	"github.com/muandane/saturdai/internal/aggregate"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func updateNodeSketches(
	log logr.Logger,
	st *autosizev1.ProfileContainerStatus,
	perNode map[string]nodeUsageSample,
	now time.Time,
) {
	if len(perNode) == 0 {
		st.Stats.NodeSketches = nil
		return
	}

	byName := indexNodeSketchEntries(st.Stats.NodeSketches)
	for node, sample := range perNode {
		idx, ok := byName[node]
		if !ok {
			st.Stats.NodeSketches = append(st.Stats.NodeSketches, autosizev1.NodeSketchEntry{NodeName: node})
			idx = len(st.Stats.NodeSketches) - 1
			byName[node] = idx
		}
		e := &st.Stats.NodeSketches[idx]
		ts := metav1.NewTime(now)
		e.LastSeen = &ts

		if err := aggregate.UpdateSketchOnly(aggregate.ResourceSample{
			Value:     sample.CPUMilli,
			GetSketch: func() string { return e.CPUSketch },
			SetSketch: func(v string) { e.CPUSketch = v },
			GetEMA:    func() (float64, float64) { return 0, 0 },
			SetEMA:    func(_, _ float64) {},
			OnAddError: func(err error) {
				log.Info("node cpu sketch add", "node", node, "error", err)
			},
		}); err != nil {
			log.Info("node cpu sketch", "node", node, "error", err)
		}
		if err := aggregate.UpdateSketchOnly(aggregate.ResourceSample{
			Value:     sample.MemBytes,
			GetSketch: func() string { return e.MemSketch },
			SetSketch: func(v string) { e.MemSketch = v },
			GetEMA:    func() (float64, float64) { return 0, 0 },
			SetEMA:    func(_, _ float64) {},
			OnAddError: func(err error) {
				log.Info("node mem sketch add", "node", node, "error", err)
			},
		}); err != nil {
			log.Info("node mem sketch", "node", node, "error", err)
		}
	}

	// Drop nodes that no longer host pods for this workload.
	out := st.Stats.NodeSketches[:0]
	for i := range st.Stats.NodeSketches {
		e := st.Stats.NodeSketches[i]
		if _, ok := perNode[e.NodeName]; !ok {
			continue
		}
		out = append(out, e)
	}
	st.Stats.NodeSketches = out

	st.Stats.NodeSketches = capNodeSketches(st.Stats.NodeSketches, autosizev1.MaxNodeSketches)
}

func indexNodeSketchEntries(entries []autosizev1.NodeSketchEntry) map[string]int {
	m := make(map[string]int, len(entries))
	for i := range entries {
		m[entries[i].NodeName] = i
	}
	return m
}

func capNodeSketches(entries []autosizev1.NodeSketchEntry, max int) []autosizev1.NodeSketchEntry {
	if len(entries) <= max {
		return entries
	}
	sort.SliceStable(entries, func(i, j int) bool {
		ti := lastSeenTime(entries[i].LastSeen)
		tj := lastSeenTime(entries[j].LastSeen)
		return ti.Before(tj)
	})
	return entries[len(entries)-max:]
}

func lastSeenTime(t *metav1.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return t.Time
}

func clearNodeSketchesCPU(entries []autosizev1.NodeSketchEntry) {
	for i := range entries {
		entries[i].CPUSketch = ""
	}
}

func clearNodeSketchesMem(entries []autosizev1.NodeSketchEntry) {
	for i := range entries {
		entries[i].MemSketch = ""
	}
}
