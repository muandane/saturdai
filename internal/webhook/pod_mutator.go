// Package webhook implements mutating admission for Pods (LLD-110).
package webhook

import (
	"context"
	"encoding/json"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	autosizev1 "github.com/muandane/saturdai/api/v1"
	"github.com/muandane/saturdai/internal/defaults"
)

const (
	annoWebhookDisabled = "autosize.io/webhook"
	annoInject          = "autosize.io/inject"
	valDisabled         = "disabled"
	valForce            = "force"
)

// PodMutator injects resources on Pod create from WorkloadProfile status or global defaults.
type PodMutator struct {
	Client   client.Client
	Decoder  admission.Decoder
	Defaults defaults.GlobalDefaultsStore
}

// Handle implements admission.Handler.
func (m *PodMutator) Handle(ctx context.Context, req admission.Request) admission.Response {
	if req.Operation != admissionv1.Create {
		return admission.Allowed("not create")
	}

	pod := &corev1.Pod{}
	if err := m.Decoder.Decode(req, pod); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if pod.Annotations != nil && pod.Annotations[annoWebhookDisabled] == valDisabled {
		return admission.Allowed("webhook disabled by annotation")
	}
	force := pod.Annotations != nil && pod.Annotations[annoInject] == valForce

	kind, wname, ok := m.resolveWorkload(ctx, pod)
	if !ok {
		return admission.Allowed("no supported workload owner")
	}

	var profile *autosizev1.WorkloadProfile
	wpList := &autosizev1.WorkloadProfileList{}
	if err := m.Client.List(ctx, wpList, client.InNamespace(pod.Namespace)); err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	for i := range wpList.Items {
		wp := &wpList.Items[i]
		if wp.Spec.TargetRef.Kind == kind && wp.Spec.TargetRef.Name == wname {
			profile = wp
			break
		}
	}

	recByName := map[string]autosizev1.Recommendation{}
	if profile != nil {
		for _, r := range profile.Status.Recommendations {
			recByName[r.ContainerName] = r
		}
	}

	gd := m.defaultsSnapshot()
	changed := false
	for i := range pod.Spec.Containers {
		c := &pod.Spec.Containers[i]
		if !force && hasNonZeroResources(*c) {
			continue
		}
		rec, hasRec := recByName[c.Name]
		switch {
		case hasRec:
			applyRecommendation(c, rec)
			changed = true
		case gd != nil:
			applyGlobalDefaults(c, gd)
			changed = true
		}
	}

	if !changed {
		return admission.Allowed("no injection")
	}

	marshaled, err := json.Marshal(pod)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, marshaled)
}

func (m *PodMutator) defaultsSnapshot() *defaults.GlobalResourceDefaults {
	if m.Defaults == nil {
		return nil
	}
	return m.Defaults.Snapshot()
}

func hasNonZeroResources(c corev1.Container) bool {
	for _, q := range c.Resources.Requests {
		if !q.IsZero() {
			return true
		}
	}
	for _, q := range c.Resources.Limits {
		if !q.IsZero() {
			return true
		}
	}
	return false
}

func applyRecommendation(c *corev1.Container, rec autosizev1.Recommendation) {
	if c.Resources.Requests == nil {
		c.Resources.Requests = corev1.ResourceList{}
	}
	if c.Resources.Limits == nil {
		c.Resources.Limits = corev1.ResourceList{}
	}
	c.Resources.Requests[corev1.ResourceCPU] = rec.CPURequest.DeepCopy()
	c.Resources.Requests[corev1.ResourceMemory] = rec.MemoryRequest.DeepCopy()
	c.Resources.Limits[corev1.ResourceCPU] = rec.CPULimit.DeepCopy()
	c.Resources.Limits[corev1.ResourceMemory] = rec.MemoryLimit.DeepCopy()
}

func applyGlobalDefaults(c *corev1.Container, gd *defaults.GlobalResourceDefaults) {
	if c.Resources.Requests == nil {
		c.Resources.Requests = corev1.ResourceList{}
	}
	if c.Resources.Limits == nil {
		c.Resources.Limits = corev1.ResourceList{}
	}
	c.Resources.Requests[corev1.ResourceCPU] = gd.CPURequest.DeepCopy()
	c.Resources.Requests[corev1.ResourceMemory] = gd.MemoryRequest.DeepCopy()
	c.Resources.Limits[corev1.ResourceCPU] = gd.CPULimit.DeepCopy()
	c.Resources.Limits[corev1.ResourceMemory] = gd.MemoryLimit.DeepCopy()
}

// resolveWorkload returns apps/v1 Deployment or StatefulSet kind and name for the pod.
func (m *PodMutator) resolveWorkload(ctx context.Context, pod *corev1.Pod) (kind, name string, ok bool) {
	ns := pod.Namespace
	for _, ref := range pod.OwnerReferences {
		if ref.APIVersion == "apps/v1" && ref.Kind == "ReplicaSet" {
			rs := &appsv1.ReplicaSet{}
			if err := m.Client.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: ns}, rs); err != nil {
				return "", "", false
			}
			for i := range rs.OwnerReferences {
				p := &rs.OwnerReferences[i]
				if p.APIVersion == "apps/v1" && p.Kind == "Deployment" {
					return "Deployment", p.Name, true
				}
			}
			return "", "", false
		}
		if ref.APIVersion == "apps/v1" && ref.Kind == "StatefulSet" {
			return "StatefulSet", ref.Name, true
		}
	}
	return "", "", false
}
