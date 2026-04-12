package controller

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	autosizev1 "github.com/muandane/saturdai/api/v1"
	"github.com/muandane/saturdai/internal/actuate"
	"github.com/muandane/saturdai/internal/aggregate"
	"github.com/muandane/saturdai/internal/defaults"
	"github.com/muandane/saturdai/internal/kubelet"
	"github.com/muandane/saturdai/internal/mlstate"
	"github.com/muandane/saturdai/internal/podsignals"
	"github.com/muandane/saturdai/internal/recommend"
	"github.com/muandane/saturdai/internal/resourcecanonical"
	"github.com/muandane/saturdai/internal/safety"
	"github.com/muandane/saturdai/internal/target"

	"github.com/DataDog/sketches-go/ddsketch"
)

func actuationEnabled() bool {
	v := os.Getenv("AUTOSIZE_ACTUATION")
	return v == "true" || v == "1"
}

func requeueAfter(profile *autosizev1.WorkloadProfile) time.Duration {
	return time.Duration(defaults.CollectionInterval(profile.Spec)) * time.Second
}

// metricsRequeueAfter is used when kubelet stats are unavailable: backoff at least 10s (spec §12).
func metricsRequeueAfter(profile *autosizev1.WorkloadProfile) time.Duration {
	d := requeueAfter(profile)
	if d < 10*time.Second {
		return 10 * time.Second
	}
	return d
}

// kubeletSummariesFullyUnavailable is true when pods are scheduled to nodes but every
// kubelet summary fetch failed (no usable stats for this reconcile).
func kubeletSummariesFullyUnavailable(nodeCount, summaryCount int) bool {
	return nodeCount > 0 && summaryCount == 0
}

func (r *WorkloadProfileReconciler) now() time.Time {
	if r.Clock != nil {
		return r.Clock()
	}
	return time.Now()
}

func (r *WorkloadProfileReconciler) reconcile(ctx context.Context, profile *autosizev1.WorkloadProfile) (*time.Duration, error) {
	logger := log.FromContext(ctx)
	ns := profile.Namespace
	mode := defaults.EffectiveMode(profile.Spec)
	lastEvaluatedBefore := profile.Status.LastEvaluated
	baselineSeen := lastEvaluatedBefore != nil
	pauseRemaining := profile.Status.DownsizePauseCyclesRemaining

	obj, err := r.Target.Resolve(ctx, ns, profile.Spec.TargetRef.Kind, profile.Spec.TargetRef.Name)
	if err != nil {
		if target.IsNotFound(err) {
			setCondition(profile, autosizev1.ConditionTypeTargetResolved, metav1.ConditionFalse, "NotFound", "target workload not found")
			syncProfileReady(profile)
			if uerr := r.persistStatus(ctx, profile); uerr != nil {
				return nil, uerr
			}
			return nil, nil
		}
		setCondition(profile, autosizev1.ConditionTypeTargetResolved, metav1.ConditionFalse, "Error", err.Error())
		syncProfileReady(profile)
		if uerr := r.persistStatus(ctx, profile); uerr != nil {
			return nil, uerr
		}
		return nil, err
	}
	setCondition(profile, autosizev1.ConditionTypeTargetResolved, metav1.ConditionTrue, "Resolved", "target found")

	sel, err := target.Selector(obj)
	if err != nil {
		return nil, fmt.Errorf("selector: %w", err)
	}

	pods := &corev1.PodList{}
	if err := r.Client.List(ctx, pods, client.InNamespace(ns), client.MatchingLabelsSelector{Selector: sel}); err != nil {
		return nil, fmt.Errorf("list pods: %w", err)
	}

	sig := podsignals.NewSnapshot()
	for i := range pods.Items {
		sig.MergePodStatus(&pods.Items[i])
	}

	summaries := map[string]*kubelet.Summary{}
	nodes := map[string]struct{}{}
	for i := range pods.Items {
		if pods.Items[i].Spec.NodeName != "" {
			nodes[pods.Items[i].Spec.NodeName] = struct{}{}
		}
	}
	var fetchErrs []string
	for n := range nodes {
		sum, err := r.Kubelet.FetchSummary(ctx, n)
		if err != nil {
			logger.Info("kubelet summary failed", "node", n, "error", err)
			fetchErrs = append(fetchErrs, fmt.Sprintf("%s: %v", n, err))
			continue
		}
		summaries[n] = sum
	}
	if kubeletSummariesFullyUnavailable(len(nodes), len(summaries)) {
		return r.persistKubeletUnavailable(ctx, profile, fetchErrs)
	}

	return r.runObserveAndActuate(ctx, profile, obj, pods.Items, summaries, sig, mode, baselineSeen, pauseRemaining)
}

func (r *WorkloadProfileReconciler) persistKubeletUnavailable(ctx context.Context, profile *autosizev1.WorkloadProfile, fetchErrs []string) (*time.Duration, error) {
	msg := strings.Join(fetchErrs, "; ")
	if msg == "" {
		msg = "kubelet summary fetch failed for all nodes with scheduled pods"
	}
	setCondition(profile, autosizev1.ConditionTypeMetricsAvailable, metav1.ConditionFalse, "KubeletUnavailable", msg)
	syncProfileReady(profile)
	if uerr := r.persistStatus(ctx, profile); uerr != nil {
		return nil, uerr
	}
	d := metricsRequeueAfter(profile)
	return &d, nil
}

func (r *WorkloadProfileReconciler) runObserveAndActuate(
	ctx context.Context,
	profile *autosizev1.WorkloadProfile,
	obj runtime.Object,
	pods []corev1.Pod,
	summaries map[string]*kubelet.Summary,
	sig *podsignals.Snapshot,
	mode string,
	baselineSeen bool,
	pauseRemaining int32,
) (*time.Duration, error) {
	logger := log.FromContext(ctx)
	ns := profile.Namespace
	tplNames, err := target.TemplateContainerNames(obj)
	if err != nil {
		return nil, err
	}

	mlState := mlstate.New()
	if r.MLState != nil {
		loaded, loadErr := r.MLState.Load(ctx, profile)
		if loadErr != nil {
			return nil, loadErr
		}
		mlState = loaded
	}

	byName := indexContainerStatus(profile.Status.Containers)
	var anySpike bool
	var maxHetero float64
	forecasts := make(map[string]struct{ CPU, Mem float64 }, len(tplNames))
	for _, cname := range tplNames {
		st := byName[cname]

		perNode, cpuMilli, memBytes, throttled, usage := collectUsageBreakdown(summaries, ns, pods, cname)
		if h := cpuHeteroScore(perNode); h > maxHetero {
			maxHetero = h
		}
		if throttled > 0 && usage > 0 {
			sig.SetThrottleRatio(cname, throttled, usage)
		}

		fCPU, fMem, err := r.ingestContainerMetrics(logger, profile, mlState, &st, cname, cpuMilli, memBytes, perNode)
		if err != nil {
			return nil, err
		}
		forecasts[cname] = struct{ CPU, Mem float64 }{CPU: fCPU, Mem: fMem}

		observedAt := metav1.NewTime(r.now())
		st.Stats.CPU.LastUpdated = &observedAt
		st.Stats.Memory.LastUpdated = &observedAt

		applyLastOOMKillFromSnapshot(&st, sig.LastOOMKill[cname])

		prevRestart := st.Stats.RestartCount
		currentMax := sig.RestartCount[cname]
		st.Stats.RestartCount = currentMax
		if isRestartSpike(prevRestart, currentMax, baselineSeen) {
			anySpike = true
		}

		byName[cname] = st
	}

	nodeSet := map[string]struct{}{}
	for i := range pods {
		if pods[i].Namespace != ns || pods[i].Spec.NodeName == "" {
			continue
		}
		nodeSet[pods[i].Spec.NodeName] = struct{}{}
	}
	bpAt := metav1.NewTime(r.now())
	profile.Status.BinPacking = &autosizev1.BinPackingHints{
		HeteroScore: maxHetero,
		NodeCount:   int32(len(nodeSet)),
		ObservedAt:  &bpAt,
	}

	pruneMLState(mlState, tplNames)

	profile.Status.Containers = flattenContainerStatus(byName, tplNames)

	minMax := overridesMap(profile)

	bias := recommend.NewLiveBias(mlState.Feedback)
	engine := recommend.New(mode, bias)
	var recs []autosizev1.Recommendation
	for _, cname := range tplNames {
		st := byName[cname]
		cpuSketch, memSketch := loadSketchesForRecommend(&st)
		quad := utcQuadrantIndex(r.now())
		quadCPU, _ := aggregate.SketchFromBase64(quadSketchGet(st.Stats.CPU.QuadrantSketches, quad))
		quadMem, _ := aggregate.SketchFromBase64(quadSketchGet(st.Stats.Memory.QuadrantSketches, quad))
		fc := forecasts[cname]
		mm := minMax[cname]
		rec, err := engine.Compute(recommend.Input{
			ContainerName:     cname,
			Mode:              mode,
			CPUSketch:         cpuSketch,
			MemSketch:         memSketch,
			QuadrantCPUSketch: quadCPU,
			QuadrantMemSketch: quadMem,
			CPUEShort:         st.Stats.CPU.EMAShort,
			CPUELong:          st.Stats.CPU.EMALong,
			MemShort:          st.Stats.Memory.EMAShort,
			MemLong:           st.Stats.Memory.EMALong,
			ForecastCPU:       fc.CPU,
			ForecastMem:       fc.Mem,
			MinCPU:            mm.MinCPU,
			MaxCPU:            mm.MaxCPU,
			MinMemory:         mm.MinMemory,
			MaxMemory:         mm.MaxMemory,
		})
		if err != nil {
			return nil, err
		}
		recs = append(recs, rec)
	}

	metricsRecs, err := resourcecanonical.CanonicalizeRecommendations(recs)
	if err != nil {
		return nil, fmt.Errorf("canonicalize metrics recommendations: %w", err)
	}
	profile.Status.MetricsRecommendations = append([]autosizev1.Recommendation(nil), metricsRecs...)

	curRes, err := currentResourcesFromTemplate(obj, tplNames)
	if err != nil {
		return nil, err
	}

	blockDownsize := pauseRemaining > 0 || anySpike
	safe := safety.Apply(profile, recs, curRes, sig, r.now(), blockDownsize)
	finalRecs, err := resourcecanonical.CanonicalizeRecommendations(safe.Recommendations)
	if err != nil {
		return nil, fmt.Errorf("canonicalize recommendations: %w", err)
	}
	profile.Status.Recommendations = finalRecs
	profile.Status.DownsizePauseCyclesRemaining = restartPauseAfterReconcile(baselineSeen, anySpike, pauseRemaining)
	lastEval := metav1.NewTime(r.now())
	profile.Status.LastEvaluated = &lastEval
	setCondition(profile, autosizev1.ConditionTypeMetricsAvailable, metav1.ConditionTrue, "Collected", "metrics processed")
	syncProfileReady(profile)

	if err := r.persistStatus(ctx, profile); err != nil {
		return nil, err
	}
	if err := r.saveMLState(ctx, profile, mlState); err != nil {
		return nil, err
	}

	if !actuationEnabled() || !safe.ShouldPatch {
		return nil, nil
	}

	if err := actuate.Apply(ctx, r.Client, obj, finalRecs, safe.SkipMemory); err != nil {
		return nil, err
	}
	profile.Status.LastApplied = &metav1.Time{Time: r.now()}
	if err := r.persistStatus(ctx, profile); err != nil {
		return nil, err
	}
	return nil, nil
}

type minMax struct {
	MinCPU, MaxCPU, MinMemory, MaxMemory *resource.Quantity
}

func overridesMap(profile *autosizev1.WorkloadProfile) map[string]minMax {
	out := map[string]minMax{}
	for _, c := range profile.Spec.Containers {
		out[c.Name] = minMax{
			MinCPU: c.MinCPU, MaxCPU: c.MaxCPU,
			MinMemory: c.MinMemory, MaxMemory: c.MaxMemory,
		}
	}
	return out
}

func indexContainerStatus(items []autosizev1.ProfileContainerStatus) map[string]autosizev1.ProfileContainerStatus {
	out := make(map[string]autosizev1.ProfileContainerStatus)
	for _, c := range items {
		out[c.Name] = c
	}
	return out
}

func flattenContainerStatus(byName map[string]autosizev1.ProfileContainerStatus, order []string) []autosizev1.ProfileContainerStatus {
	out := make([]autosizev1.ProfileContainerStatus, 0, len(order))
	for _, n := range order {
		out = append(out, byName[n])
	}
	return out
}

func loadSketchesForRecommend(st *autosizev1.ProfileContainerStatus) (*ddsketch.DDSketch, *ddsketch.DDSketch) {
	cpuEnc, memEnc := sketchBase64sFromNodes(st)
	cpu, _ := aggregate.SketchFromBase64(st.Stats.CPU.Sketch)
	mem, _ := aggregate.SketchFromBase64(st.Stats.Memory.Sketch)
	if len(cpuEnc) > 0 {
		if merged, err := aggregate.MergeSketchesFromBase64(cpuEnc); err == nil && merged != nil && !merged.IsEmpty() {
			cpu = merged
		}
	}
	if len(memEnc) > 0 {
		if merged, err := aggregate.MergeSketchesFromBase64(memEnc); err == nil && merged != nil && !merged.IsEmpty() {
			mem = merged
		}
	}
	return cpu, mem
}

func sketchBase64sFromNodes(st *autosizev1.ProfileContainerStatus) (cpu, mem []string) {
	for i := range st.Stats.NodeSketches {
		e := st.Stats.NodeSketches[i]
		if e.CPUSketch != "" {
			cpu = append(cpu, e.CPUSketch)
		}
		if e.MemSketch != "" {
			mem = append(mem, e.MemSketch)
		}
	}
	return
}

func currentResourcesFromTemplate(obj runtime.Object, names []string) (map[string]corev1.ResourceRequirements, error) {
	out := map[string]corev1.ResourceRequirements{}
	switch t := obj.(type) {
	case *appsv1.Deployment:
		for i := range t.Spec.Template.Spec.Containers {
			c := t.Spec.Template.Spec.Containers[i]
			for _, n := range names {
				if c.Name == n {
					out[n] = c.Resources
				}
			}
		}
		return out, nil
	case *appsv1.StatefulSet:
		for i := range t.Spec.Template.Spec.Containers {
			c := t.Spec.Template.Spec.Containers[i]
			for _, n := range names {
				if c.Name == n {
					out[n] = c.Resources
				}
			}
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported target %T", obj)
	}
}

func setCondition(profile *autosizev1.WorkloadProfile, typ string, status metav1.ConditionStatus, reason, message string) {
	c := metav1.Condition{
		Type:               typ,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: profile.Generation,
		LastTransitionTime: metav1.Now(),
	}
	for i := range profile.Status.Conditions {
		if profile.Status.Conditions[i].Type == typ {
			profile.Status.Conditions[i] = c
			return
		}
	}
	profile.Status.Conditions = append(profile.Status.Conditions, c)
}

func conditionStatus(profile *autosizev1.WorkloadProfile, typ string) metav1.ConditionStatus {
	for i := range profile.Status.Conditions {
		if profile.Status.Conditions[i].Type == typ {
			return profile.Status.Conditions[i].Status
		}
	}
	return metav1.ConditionFalse
}

func conditionMessage(profile *autosizev1.WorkloadProfile, typ string) string {
	for i := range profile.Status.Conditions {
		if profile.Status.Conditions[i].Type == typ {
			return profile.Status.Conditions[i].Message
		}
	}
	return ""
}

// syncProfileReady sets ProfileReady to True iff TargetResolved and MetricsAvailable are True.
func syncProfileReady(profile *autosizev1.WorkloadProfile) {
	targetOK := conditionStatus(profile, autosizev1.ConditionTypeTargetResolved) == metav1.ConditionTrue
	metricsOK := conditionStatus(profile, autosizev1.ConditionTypeMetricsAvailable) == metav1.ConditionTrue

	if targetOK && metricsOK {
		setCondition(profile, autosizev1.ConditionTypeProfileReady, metav1.ConditionTrue, "Ready", "target resolved and metrics available")
		return
	}
	var reason, msg string
	if !targetOK {
		reason = "TargetNotReady"
		msg = conditionMessage(profile, autosizev1.ConditionTypeTargetResolved)
		if msg == "" {
			msg = "target workload not ready"
		}
	} else {
		reason = "MetricsNotAvailable"
		msg = conditionMessage(profile, autosizev1.ConditionTypeMetricsAvailable)
		if msg == "" {
			msg = "metrics not yet available"
		}
	}
	setCondition(profile, autosizev1.ConditionTypeProfileReady, metav1.ConditionFalse, reason, msg)
}
