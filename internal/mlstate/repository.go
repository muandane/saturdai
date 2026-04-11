package mlstate

import (
	"context"
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	autosizev1 "github.com/muandane/saturdai/api/v1"
	"github.com/muandane/saturdai/internal/recommend"
)

// Repository persists MLState for a WorkloadProfile.
type Repository interface {
	Load(ctx context.Context, profile *autosizev1.WorkloadProfile) (*MLState, error)
	Save(ctx context.Context, profile *autosizev1.WorkloadProfile, state *MLState) error
}

// ConfigMapRepository stores MLState as JSON in a ConfigMap (name mlstate-<profile.Name>).
type ConfigMapRepository struct {
	client client.Client
}

// NewConfigMapRepository constructs a repository.
func NewConfigMapRepository(c client.Client) *ConfigMapRepository {
	return &ConfigMapRepository{client: c}
}

func mlstateConfigMapName(profileName string) string {
	return "mlstate-" + profileName
}

func controllerOwnerRef(profile *autosizev1.WorkloadProfile) metav1.OwnerReference {
	t := true
	return metav1.OwnerReference{
		APIVersion:         autosizev1.GroupVersion.String(),
		Kind:               "WorkloadProfile",
		Name:               profile.Name,
		UID:                profile.UID,
		Controller:         &t,
		BlockOwnerDeletion: &t,
	}
}

// Load implements Repository.
func (r *ConfigMapRepository) Load(ctx context.Context, profile *autosizev1.WorkloadProfile) (*MLState, error) {
	cm := &corev1.ConfigMap{}
	key := types.NamespacedName{
		Namespace: profile.Namespace,
		Name:      mlstateConfigMapName(profile.Name),
	}
	if err := r.client.Get(ctx, key, cm); err != nil {
		if apierrors.IsNotFound(err) {
			return New(), nil
		}
		return nil, err
	}
	state := New()
	raw, ok := cm.Data["state"]
	if !ok || raw == "" {
		return state, nil
	}
	if err := json.Unmarshal([]byte(raw), state); err != nil {
		return New(), nil
	}
	if state.CUSUM == nil {
		state.CUSUM = map[string]*ContainerCUSUM{}
	}
	if state.Feedback == nil {
		state.Feedback = map[string]*recommend.ContainerFeedback{}
	}
	if state.HW == nil {
		state.HW = map[string]*ContainerHW{}
	}
	return state, nil
}

// Save implements Repository.
func (r *ConfigMapRepository) Save(ctx context.Context, profile *autosizev1.WorkloadProfile, state *MLState) error {
	if state == nil {
		state = New()
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	name := mlstateConfigMapName(profile.Name)
	cm := &corev1.ConfigMap{}
	key := types.NamespacedName{Namespace: profile.Namespace, Name: name}
	err = r.client.Get(ctx, key, cm)
	if apierrors.IsNotFound(err) {
		cm = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:            name,
				Namespace:       profile.Namespace,
				OwnerReferences: []metav1.OwnerReference{controllerOwnerRef(profile)},
			},
			Data: map[string]string{"state": string(raw)},
		}
		return r.client.Create(ctx, cm)
	}
	if err != nil {
		return err
	}
	patch := cm.DeepCopy()
	patch.Data = map[string]string{"state": string(raw)}
	return r.client.Patch(ctx, patch, client.MergeFrom(cm))
}
