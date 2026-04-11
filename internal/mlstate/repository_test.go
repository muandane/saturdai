package mlstate

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	autosizev1 "github.com/muandane/saturdai/api/v1"
)

func TestConfigMapRepository_roundTrip(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := autosizev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	profile := &autosizev1.WorkloadProfile{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "wp1",
			Namespace:       "ns1",
			UID:             "uid-wp1",
			ResourceVersion: "1",
		},
		Spec: autosizev1.WorkloadProfileSpec{
			TargetRef: autosizev1.WorkloadTargetRef{Kind: "Deployment", Name: "d1"},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(profile).Build()
	repo := NewConfigMapRepository(cl)

	ctx := context.Background()
	st, err := repo.Load(ctx, profile)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.CUSUM) != 0 {
		t.Fatal("expected empty")
	}

	st.CUSUM["app"] = &ContainerCUSUM{}
	if err := repo.Save(ctx, profile, st); err != nil {
		t.Fatal(err)
	}

	st2, err := repo.Load(ctx, profile)
	if err != nil {
		t.Fatal(err)
	}
	if st2.CUSUM["app"] == nil {
		t.Fatal("expected cusum key")
	}
}
