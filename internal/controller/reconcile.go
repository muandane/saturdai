package controller

import (
	"context"
	"fmt"
	"os"
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
	"github.com/muandane/saturdai/internal/podsignals"
	"github.com/muandane/saturdai/internal/recommend"
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

func (r *WorkloadProfileReconciler) reconcile(ctx context.Context, profile *autosizev1.WorkloadProfile) error {
	logger := log.FromContext(ctx)
	ns := profile.Namespace
	mode := defaults.EffectiveMode(profile.Spec)

	obj, err := r.Target.Resolve(ctx, ns, profile.Spec.TargetRef.Kind, profile.Spec.TargetRef.Name)
	if err != nil {
		if target.IsNotFound(err) {
			setCondition(profile, autosizev1.ConditionTypeTargetResolved, metav1.ConditionFalse, "NotFound", "target workload not found")
			return r.Client.Status().Update(ctx, profile)
		}
		setCondition(profile, autosizev1.ConditionTypeTargetResolved, metav1.ConditionFalse, "Error", err.Error())
		_ = r.Client.Status().Update(ctx, profile)
		return err
	}
	setCondition(profile, autosizev1.ConditionTypeTargetResolved, metav1.ConditionTrue, "Resolved", "target found")

	sel, err := target.Selector(obj)
	if err != nil {
		return fmt.Errorf("selector: %w", err)
	}

	pods := &corev1.PodList{}
	if err := r.Client.List(ctx, pods, client.InNamespace(ns), client.MatchingLabelsSelector{Selector: sel}); err != nil {
		return fmt.Errorf("list pods: %w", err)
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
	for n := range nodes {
		sum, err := r.Kubelet.FetchSummary(ctx, n)
		if err != nil {
			logger.Info("kubelet summary failed", "node", n, "error", err)
			continue
		}
		summaries[n] = sum
	}

	tplNames, err := target.TemplateContainerNames(obj)
	if err != nil {
		return err
	}

	byName := indexContainerStatus(profile.Status.Containers)
	for _, cname := range tplNames {
		st := byName[cname]
		cpuSketch, memSketch := loadSketches(&st)

		cpuMilli, memBytes, throttled, usage := collectUsageForContainer(summaries, ns, pods.Items, cname)
		if throttled > 0 && usage > 0 {
			sig.SetThrottleRatio(cname, throttled, usage)
		}

		st.Name = cname
		if cpuMilli > 0 {
			st.Stats.CPU.EMAShort, st.Stats.CPU.EMALong = aggregate.UpdateEMA(st.Stats.CPU.EMAShort, st.Stats.CPU.EMALong, cpuMilli)
		}
		if memBytes > 0 {
			st.Stats.Memory.EMAShort, st.Stats.Memory.EMALong = aggregate.UpdateEMA(st.Stats.Memory.EMAShort, st.Stats.Memory.EMALong, memBytes)
		}
		st.Stats.Memory.SlopePositive = st.Stats.Memory.EMAShort > st.Stats.Memory.EMALong*1.01

		if err := cpuSketch.Add(cpuMilli); err != nil {
			logger.Info("cpu sketch add", "error", err)
		}
		if err := memSketch.Add(memBytes); err != nil {
			logger.Info("mem sketch add", "error", err)
		}
		cs, err := aggregate.SketchToBase64(cpuSketch)
		if err != nil {
			return err
		}
		ms, err := aggregate.SketchToBase64(memSketch)
		if err != nil {
			return err
		}
		st.Stats.CPU.Sketch = cs
		st.Stats.Memory.Sketch = ms
		now := metav1.Now()
		st.Stats.CPU.LastUpdated = &now
		st.Stats.Memory.LastUpdated = &now

		byName[cname] = st
	}

	profile.Status.Containers = flattenContainerStatus(byName, tplNames)

	minMax := overridesMap(profile)

	var recs []autosizev1.Recommendation
	for _, cname := range tplNames {
		st := byName[cname]
		cpuSketch, memSketch := loadSketches(&st)
		mm := minMax[cname]
		rec, err := recommend.Compute(recommend.Input{
			ContainerName: cname,
			Mode:          mode,
			CPUSketch:     cpuSketch,
			MemSketch:     memSketch,
			CPUEShort:     st.Stats.CPU.EMAShort,
			CPUELong:      st.Stats.CPU.EMALong,
			MemShort:      st.Stats.Memory.EMAShort,
			MemLong:       st.Stats.Memory.EMALong,
			MinCPU:        mm.MinCPU,
			MaxCPU:        mm.MaxCPU,
			MinMemory:     mm.MinMemory,
			MaxMemory:     mm.MaxMemory,
		})
		if err != nil {
			return err
		}
		recs = append(recs, rec)
	}

	curRes, err := currentResourcesFromTemplate(obj, tplNames)
	if err != nil {
		return err
	}

	safe := safety.Apply(profile, recs, curRes, sig, time.Now())
	profile.Status.Recommendations = safe.Recommendations
	t := metav1.Now()
	profile.Status.LastEvaluated = &t
	setCondition(profile, autosizev1.ConditionTypeMetricsAvailable, metav1.ConditionTrue, "Collected", "metrics processed")

	if err := r.Client.Status().Update(ctx, profile); err != nil {
		return err
	}

	if !actuationEnabled() || !safe.ShouldPatch {
		return nil
	}

	if err := actuate.Apply(ctx, r.Client, obj, safe.Recommendations, safe.SkipMemory); err != nil {
		return err
	}
	profile.Status.LastApplied = &metav1.Time{Time: time.Now()}
	return r.Client.Status().Update(ctx, profile)
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

func loadSketches(st *autosizev1.ProfileContainerStatus) (*ddsketch.DDSketch, *ddsketch.DDSketch) {
	cpu, _ := aggregate.SketchFromBase64(st.Stats.CPU.Sketch)
	mem, _ := aggregate.SketchFromBase64(st.Stats.Memory.Sketch)
	return cpu, mem
}

func collectUsageForContainer(summaries map[string]*kubelet.Summary, ns string, pods []corev1.Pod, container string) (cpuMilli float64, memBytes float64, throttled uint64, usage uint64) {
	var cpuSum, memSum float64
	var cpuN, memN int
	var thr, use uint64
	for _, pod := range pods {
		if pod.Namespace != ns {
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
				if cs.CPU != nil && cs.CPU.UsageNanoCores != nil {
					cpuSum += float64(*cs.CPU.UsageNanoCores) / 1e6
					cpuN++
					if cs.CPU.ThrottledUsageNanoCores != nil {
						thr += *cs.CPU.ThrottledUsageNanoCores
					}
					if cs.CPU.UsageNanoCores != nil {
						use += *cs.CPU.UsageNanoCores
					}
				}
				if cs.Memory != nil && cs.Memory.WorkingSetBytes != nil {
					memSum += float64(*cs.Memory.WorkingSetBytes)
					memN++
				}
			}
		}
	}
	if cpuN > 0 {
		cpuMilli = cpuSum / float64(cpuN)
	}
	if memN > 0 {
		memBytes = memSum / float64(memN)
	}
	return cpuMilli, memBytes, thr, use
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
