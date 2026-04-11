package dashboard

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	autosizev1 "github.com/muandane/saturdai/api/v1"
	"github.com/muandane/saturdai/internal/changepoint"
	"github.com/muandane/saturdai/internal/mlstate"
)

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := appsv1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := autosizev1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestBuildProfileView_mapsStatusAndCUSUM(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	scheme := testScheme(t)

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "x"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "x"}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "app",
							Image: "nginx",
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("128Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("200m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
							},
						},
					},
				},
			},
		},
	}

	wp := &autosizev1.WorkloadProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "wp1", Namespace: "default", UID: "uid-1"},
		Spec: autosizev1.WorkloadProfileSpec{
			TargetRef: autosizev1.WorkloadTargetRef{Kind: "Deployment", Name: "web"},
			Mode:      "balanced",
		},
		Status: autosizev1.WorkloadProfileStatus{
			LastEvaluated: &metav1.Time{Time: metav1.Now().Time},
			Containers: []autosizev1.ProfileContainerStatus{
				{
					Name: "app",
					Stats: autosizev1.ContainerResourceStats{
						CPU: autosizev1.CPUStats{
							EMAShort: 100,
							EMALong:  90,
						},
						Memory: autosizev1.MemoryStats{
							EMAShort:      200 * mib,
							EMALong:       180 * mib,
							SlopeStreak:   2,
							SlopePositive: false,
						},
						RestartCount: 1,
					},
				},
			},
			MetricsRecommendations: []autosizev1.Recommendation{
				{
					ContainerName: "app",
					CPURequest:    resource.MustParse("150m"),
					CPULimit:      resource.MustParse("300m"),
					MemoryRequest: resource.MustParse("256Mi"),
					MemoryLimit:   resource.MustParse("512Mi"),
					Rationale:     "mode:balanced",
				},
			},
			Recommendations: []autosizev1.Recommendation{
				{
					ContainerName: "app",
					CPURequest:    resource.MustParse("150m"),
					CPULimit:      resource.MustParse("300m"),
					MemoryRequest: resource.MustParse("256Mi"),
					MemoryLimit:   resource.MustParse("512Mi"),
					Rationale:     "mode:balanced safety:ok",
				},
			},
			Conditions: []metav1.Condition{
				{Type: autosizev1.ConditionTypeTargetResolved, Status: metav1.ConditionTrue},
				{Type: autosizev1.ConditionTypeMetricsAvailable, Status: metav1.ConditionTrue},
				{Type: autosizev1.ConditionTypeProfileReady, Status: metav1.ConditionTrue},
			},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dep, wp).Build()

	ml := mlstate.New()
	ml.CUSUM["app"] = &mlstate.ContainerCUSUM{
		CPU:    changepoint.State{SPos: 10, SNeg: 5},
		Memory: changepoint.State{SPos: 1e6, SNeg: 2e6},
	}

	pv, err := BuildProfileView(ctx, cl, wp, ml)
	if err != nil {
		t.Fatalf("BuildProfileView: %v", err)
	}
	if pv.ID != "default/wp1" {
		t.Fatalf("id: got %q", pv.ID)
	}
	if pv.TargetName != "web" {
		t.Fatalf("targetName: got %q", pv.TargetName)
	}
	if len(pv.Containers) != 1 {
		t.Fatalf("containers: %d", len(pv.Containers))
	}
	c := pv.Containers[0]
	if c.Name != "app" {
		t.Fatalf("container name: %q", c.Name)
	}
	if c.CusumCPU.SPos != 10 || c.CusumCPU.H != changepoint.DefaultCPUConfig.H {
		t.Fatalf("cusum cpu: %+v", c.CusumCPU)
	}
	if c.CurrentCPU.Req != "100m" || c.CurrentCPU.Lim != "200m" {
		t.Fatalf("current cpu: %+v", c.CurrentCPU)
	}
	if c.ThrottleRatio != nil {
		t.Fatalf("expected nil throttle")
	}
	if !pv.Conditions.ProfileReady {
		t.Fatal("expected profile ready")
	}
}

func TestQuadrantIntensities_normalizesToOne(t *testing.T) {
	t.Parallel()
	sk := []string{"", "", "", ""}
	out := quadrantIntensityCPU(sk, 100)
	for i := range out {
		if out[i] != 0 {
			t.Fatalf("quad %d: %v", i, out[i])
		}
	}
}
