package controller

import (
	"github.com/go-logr/logr"

	autosizev1 "github.com/muandane/saturdai/api/v1"
	"github.com/muandane/saturdai/internal/aggregate"
	"github.com/muandane/saturdai/internal/changepoint"
	"github.com/muandane/saturdai/internal/mlstate"
)

// ingestContainerMetrics updates sketches, EMA, CUSUM, quadrant sketches, HW, and per-node sketches for one container.
func (r *WorkloadProfileReconciler) ingestContainerMetrics(
	log logr.Logger,
	profile *autosizev1.WorkloadProfile,
	mlState *mlstate.MLState,
	st *autosizev1.ProfileContainerStatus,
	cname string,
	cpuMilli, memBytes float64,
	perNode map[string]nodeUsageSample,
) (forecastCPU, forecastMem float64, err error) {
	st.Name = cname

	if last := lastRecommendationFor(profile, cname); last != nil {
		ensureContainerFeedback(mlState, cname).RecordUsage(
			cpuMilli, float64(last.CPURequest.MilliValue()),
			memBytes, float64(last.MemoryRequest.Value()),
		)
	}

	cpuLongBefore := st.Stats.CPU.EMALong
	memLongBefore := st.Stats.Memory.EMALong

	if err := aggregate.Update(aggregate.ResourceSample{
		Value:     cpuMilli,
		GetSketch: func() string { return st.Stats.CPU.Sketch },
		SetSketch: func(v string) { st.Stats.CPU.Sketch = v },
		GetEMA:    func() (float64, float64) { return st.Stats.CPU.EMAShort, st.Stats.CPU.EMALong },
		SetEMA: func(s, l float64) {
			st.Stats.CPU.EMAShort, st.Stats.CPU.EMALong = s, l
		},
		OnAddError: func(err error) { log.Info("cpu sketch add", "error", err) },
	}); err != nil {
		return 0, 0, err
	}
	prevMemShort := st.Stats.Memory.EMAShort
	priorMemObserved := st.Stats.Memory.LastUpdated != nil
	priorSlopeStreak := st.Stats.Memory.SlopeStreak

	if err := aggregate.Update(aggregate.ResourceSample{
		Value:     memBytes,
		GetSketch: func() string { return st.Stats.Memory.Sketch },
		SetSketch: func(v string) { st.Stats.Memory.Sketch = v },
		GetEMA:    func() (float64, float64) { return st.Stats.Memory.EMAShort, st.Stats.Memory.EMALong },
		SetEMA: func(s, l float64) {
			st.Stats.Memory.EMAShort, st.Stats.Memory.EMALong = s, l
		},
		OnAddError: func(err error) { log.Info("mem sketch add", "error", err) },
	}); err != nil {
		return 0, 0, err
	}

	st.Stats.Memory.SlopeStreak, st.Stats.Memory.SlopePositive = aggregate.UpdateMemorySlope(
		prevMemShort,
		st.Stats.Memory.EMAShort,
		priorSlopeStreak,
		aggregate.DefaultMemorySlopeCycles,
		priorMemObserved,
	)

	cu := ensureContainerCUSUM(mlState, cname)
	if cu.CPU.Update(cpuMilli, cpuLongBefore, changepoint.DefaultCPUConfig) {
		if r.Detector != nil {
			r.Detector.Notify(changepoint.ShiftEvent{
				InvolvedObject: profile,
				ContainerName:  cname,
				Resource:       "cpu",
				At:             r.now(),
				OldMean:        cpuLongBefore,
				NewMean:        cpuMilli,
			})
		}
		cu.CPU.Reset()
		st.Stats.CPU.Sketch = ""
		clearQuadrantSketches(&st.Stats.CPU.QuadrantSketches)
		clearNodeSketchesCPU(st.Stats.NodeSketches)
	}
	if cu.Memory.Update(memBytes, memLongBefore, changepoint.DefaultMemConfig) {
		if r.Detector != nil {
			r.Detector.Notify(changepoint.ShiftEvent{
				InvolvedObject: profile,
				ContainerName:  cname,
				Resource:       "memory",
				At:             r.now(),
				OldMean:        memLongBefore,
				NewMean:        memBytes,
			})
		}
		cu.Memory.Reset()
		st.Stats.Memory.Sketch = ""
		clearQuadrantSketches(&st.Stats.Memory.QuadrantSketches)
		clearNodeSketchesMem(st.Stats.NodeSketches)
	}

	utc := r.now().UTC()
	quad := utcQuadrantIndex(r.now())
	if err := aggregate.Update(aggregate.ResourceSample{
		Value:     cpuMilli,
		GetSketch: func() string { return quadSketchGet(st.Stats.CPU.QuadrantSketches, quad) },
		SetSketch: func(v string) { quadSketchSet(&st.Stats.CPU.QuadrantSketches, quad, v) },
		GetEMA:    func() (float64, float64) { return 0, 0 },
		SetEMA:    func(_, _ float64) {},
		OnAddError: func(err error) {
			log.Info("cpu quadrant sketch add", "error", err)
		},
	}); err != nil {
		return 0, 0, err
	}
	if err := aggregate.Update(aggregate.ResourceSample{
		Value:     memBytes,
		GetSketch: func() string { return quadSketchGet(st.Stats.Memory.QuadrantSketches, quad) },
		SetSketch: func(v string) { quadSketchSet(&st.Stats.Memory.QuadrantSketches, quad, v) },
		GetEMA:    func() (float64, float64) { return 0, 0 },
		SetEMA:    func(_, _ float64) {},
		OnAddError: func(err error) {
			log.Info("mem quadrant sketch add", "error", err)
		},
	}); err != nil {
		return 0, 0, err
	}

	hw := ensureContainerHW(mlState, cname)
	hour := utc.Hour()
	forecastCPU = hw.CPU.Update(cpuMilli, hour)
	forecastMem = hw.Memory.Update(memBytes, hour)

	updateNodeSketches(log, st, perNode, r.now())

	return forecastCPU, forecastMem, nil
}
