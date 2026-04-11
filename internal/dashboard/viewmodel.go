package dashboard

import (
	"context"
	"fmt"
	"math"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	autosizev1 "github.com/muandane/saturdai/api/v1"
	"github.com/muandane/saturdai/internal/aggregate"
	"github.com/muandane/saturdai/internal/changepoint"
	"github.com/muandane/saturdai/internal/mlstate"
)

const mib = 1024 * 1024

const dashPlaceholder = "—"

// ProfilesResponse is the JSON envelope for GET /api/dashboard/v1/profiles.
type ProfilesResponse struct {
	Profiles []ProfileView `json:"profiles"`
}

// ProfileView is the dashboard JSON for one WorkloadProfile.
type ProfileView struct {
	ID            string          `json:"id"`
	Namespace     string          `json:"ns"`
	Name          string          `json:"name"`
	TargetName    string          `json:"targetName"`
	Kind          string          `json:"kind"`
	Mode          string          `json:"mode"`
	Cooldown      int32           `json:"cooldown"`
	Interval      int32           `json:"interval"`
	LastEvaluated string          `json:"lastEvaluated"`
	LastApplied   string          `json:"lastApplied,omitempty"`
	DownsizePause int32           `json:"downsizePause"`
	Conditions    ConditionsView  `json:"conditions"`
	Containers    []ContainerView `json:"containers"`
}

// ConditionsView mirrors the demo UI condition flags.
type ConditionsView struct {
	TargetResolved   bool `json:"targetResolved"`
	MetricsAvailable bool `json:"metricsAvailable"`
	ProfileReady     bool `json:"profileReady"`
}

// ContainerView matches fields the static dashboard JS expects.
type ContainerView struct {
	Name          string       `json:"name"`
	CPU           CPUView      `json:"cpu"`
	Memory        MemoryView   `json:"memory"`
	RestartCount  int32        `json:"restartCount"`
	LastOOMKill   *string      `json:"lastOOMKill"`
	ThrottleRatio *float64     `json:"throttleRatio"`
	CusumCPU      CusumView    `json:"cusumCPU"`
	CusumMem      CusumMemView `json:"cusumMem"`
	CurrentCPU    ResourcePair `json:"currentCPU"`
	CurrentMem    ResourcePair `json:"currentMem"`
	MetricsReco   RecoView     `json:"metricsReco"`
	SafetyReco    RecoView     `json:"safetyReco"`
	Rationale     string       `json:"rationale"`
	QuadCPU       []float64    `json:"quadCPU"`
	QuadMem       []float64    `json:"quadMem"`
}

type CPUView struct {
	EMAShort float64 `json:"emaShort"`
	EMALong  float64 `json:"emaLong"`
	P50      float64 `json:"p50"`
	P75      float64 `json:"p75"`
	P90      float64 `json:"p90"`
}

type MemoryView struct {
	EMAShort      float64 `json:"emaShort"`
	EMALong       float64 `json:"emaLong"`
	P50           float64 `json:"p50"`
	P90           float64 `json:"p90"`
	SlopeStreak   int32   `json:"slopeStreak"`
	SlopePositive bool    `json:"slopePositive"`
}

type CusumView struct {
	SPos float64 `json:"sPos"`
	SNeg float64 `json:"sNeg"`
	H    float64 `json:"h"`
	K    float64 `json:"k"`
}

type CusumMemView struct {
	SPos float64 `json:"sPos"`
	SNeg float64 `json:"sNeg"`
	H    float64 `json:"h"`
	K    float64 `json:"k"`
}

type ResourcePair struct {
	Req string `json:"req"`
	Lim string `json:"lim"`
}

type RecoView struct {
	CPUReq string `json:"cpuReq"`
	CPULim string `json:"cpuLim"`
	MemReq string `json:"memReq"`
	MemLim string `json:"memLim"`
}

func conditionTrue(profile *autosizev1.WorkloadProfile, typ string) bool {
	for i := range profile.Status.Conditions {
		if profile.Status.Conditions[i].Type == typ {
			return profile.Status.Conditions[i].Status == metav1.ConditionTrue
		}
	}
	return false
}

func derefInt32(p *int32, def int32) int32 {
	if p == nil {
		return def
	}
	return *p
}

func formatQuantity(q *resource.Quantity) string {
	if q == nil || q.IsZero() {
		return dashPlaceholder
	}
	return q.String()
}

func timePtrString(t *metav1.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.Time.UTC().Format(time.RFC3339)
}

func quantileAt(sk *string, q float64) float64 {
	if sk == nil || *sk == "" {
		return 0
	}
	s, err := aggregate.SketchFromBase64(*sk)
	if err != nil || s == nil || s.IsEmpty() {
		return 0
	}
	v, err := aggregate.Quantile(s, q)
	if err != nil {
		return 0
	}
	return v
}

func bytesToMi(b float64) float64 {
	return b / mib
}

func quadrantIntensityCPU(sketches []string, globalP50Milli float64) []float64 {
	return quadrantIntensities(sketches, globalP50Milli, func(s string) float64 {
		sk, err := aggregate.SketchFromBase64(s)
		if err != nil || sk == nil || sk.IsEmpty() {
			return 0
		}
		v, err := aggregate.Quantile(sk, 0.5)
		if err != nil {
			return 0
		}
		return v
	})
}

func quadrantIntensityMem(sketches []string, globalP50Bytes float64) []float64 {
	globalMi := bytesToMi(globalP50Bytes)
	return quadrantIntensities(sketches, globalMi, func(s string) float64 {
		sk, err := aggregate.SketchFromBase64(s)
		if err != nil || sk == nil || sk.IsEmpty() {
			return 0
		}
		v, err := aggregate.Quantile(sk, 0.5)
		if err != nil {
			return 0
		}
		return bytesToMi(v)
	})
}

func quadrantIntensities(sketches []string, globalP50 float64, p50fn func(string) float64) []float64 {
	out := make([]float64, 4)
	maxV := globalP50
	vals := make([]float64, 4)
	for i := range 4 {
		if i >= len(sketches) || sketches[i] == "" {
			continue
		}
		vals[i] = p50fn(sketches[i])
		if vals[i] > maxV {
			maxV = vals[i]
		}
	}
	if maxV < 1e-9 {
		maxV = 1
	}
	for i := range vals {
		out[i] = math.Min(1, vals[i]/maxV)
	}
	return out
}

func findReco(list []autosizev1.Recommendation, name string) (autosizev1.Recommendation, bool) {
	for i := range list {
		if list[i].ContainerName == name {
			return list[i], true
		}
	}
	return autosizev1.Recommendation{}, false
}

func recoView(r autosizev1.Recommendation) RecoView {
	return RecoView{
		CPUReq: r.CPURequest.String(),
		CPULim: r.CPULimit.String(),
		MemReq: r.MemoryRequest.String(),
		MemLim: r.MemoryLimit.String(),
	}
}

// BuildProfileView maps API objects to dashboard JSON.
func BuildProfileView(ctx context.Context, c client.Client, profile *autosizev1.WorkloadProfile, ml *mlstate.MLState) (*ProfileView, error) {
	if profile == nil {
		return nil, fmt.Errorf("nil profile")
	}
	if ml == nil {
		ml = mlstate.New()
	}

	names := make([]string, 0, len(profile.Status.Containers))
	for i := range profile.Status.Containers {
		names = append(names, profile.Status.Containers[i].Name)
	}

	resMap, err := fetchWorkloadResources(ctx, c, profile, names)
	if err != nil {
		return nil, err
	}

	mode := profile.Spec.Mode
	if mode == "" {
		mode = "balanced"
	}

	pv := &ProfileView{
		ID:            profile.Namespace + "/" + profile.Name,
		Namespace:     profile.Namespace,
		Name:          profile.Name,
		TargetName:    profile.Spec.TargetRef.Name,
		Kind:          profile.Spec.TargetRef.Kind,
		Mode:          mode,
		Cooldown:      derefInt32(profile.Spec.CooldownMinutes, 15),
		Interval:      derefInt32(profile.Spec.CollectionIntervalSeconds, 30),
		LastEvaluated: timePtrString(profile.Status.LastEvaluated),
		LastApplied:   timePtrString(profile.Status.LastApplied),
		DownsizePause: profile.Status.DownsizePauseCyclesRemaining,
		Conditions: ConditionsView{
			TargetResolved:   conditionTrue(profile, autosizev1.ConditionTypeTargetResolved),
			MetricsAvailable: conditionTrue(profile, autosizev1.ConditionTypeMetricsAvailable),
			ProfileReady:     conditionTrue(profile, autosizev1.ConditionTypeProfileReady),
		},
		Containers: make([]ContainerView, 0, len(profile.Status.Containers)),
	}

	cpuCfg := changepoint.DefaultCPUConfig
	memCfg := changepoint.DefaultMemConfig

	for _, pc := range profile.Status.Containers {
		st := pc.Stats
		skCPU := st.CPU.Sketch
		skMem := st.Memory.Sketch

		p50CPU := quantileAt(&skCPU, 0.5)
		p75CPU := quantileAt(&skCPU, 0.75)
		p90CPU := quantileAt(&skCPU, 0.90)

		p50MemB := quantileAt(&skMem, 0.5)
		p90MemB := quantileAt(&skMem, 0.90)

		quadSkCPU := st.CPU.QuadrantSketches
		quadSkMem := st.Memory.QuadrantSketches
		quadCPU := quadrantIntensityCPU(quadSkCPU, p50CPU)
		quadMem := quadrantIntensityMem(quadSkMem, p50MemB)

		var cus *mlstate.ContainerCUSUM
		if ml.CUSUM != nil {
			cus = ml.CUSUM[pc.Name]
		}

		cv := ContainerView{
			Name: pc.Name,
			CPU: CPUView{
				EMAShort: st.CPU.EMAShort,
				EMALong:  st.CPU.EMALong,
				P50:      p50CPU,
				P75:      p75CPU,
				P90:      p90CPU,
			},
			Memory: MemoryView{
				EMAShort:      bytesToMi(st.Memory.EMAShort),
				EMALong:       bytesToMi(st.Memory.EMALong),
				P50:           bytesToMi(p50MemB),
				P90:           bytesToMi(p90MemB),
				SlopeStreak:   st.Memory.SlopeStreak,
				SlopePositive: st.Memory.SlopePositive,
			},
			RestartCount:  st.RestartCount,
			ThrottleRatio: nil,
			CusumCPU: CusumView{
				SPos: 0,
				SNeg: 0,
				H:    cpuCfg.H,
				K:    cpuCfg.K,
			},
			CusumMem: CusumMemView{
				SPos: 0,
				SNeg: 0,
				H:    memCfg.H,
				K:    memCfg.K,
			},
			QuadCPU: quadCPU,
			QuadMem: quadMem,
		}

		if cus != nil {
			cv.CusumCPU.SPos = cus.CPU.SPos
			cv.CusumCPU.SNeg = cus.CPU.SNeg
			cv.CusumMem.SPos = cus.Memory.SPos
			cv.CusumMem.SNeg = cus.Memory.SNeg
		}

		if st.LastOOMKill != nil && !st.LastOOMKill.IsZero() {
			s := st.LastOOMKill.Time.UTC().Format(time.RFC3339)
			cv.LastOOMKill = &s
		}

		metricsRec, okM := findReco(profile.Status.MetricsRecommendations, pc.Name)
		safetyRec, okS := findReco(profile.Status.Recommendations, pc.Name)
		if okM {
			cv.MetricsReco = recoView(metricsRec)
		}
		if okS {
			cv.SafetyReco = recoView(safetyRec)
			cv.Rationale = safetyRec.Rationale
		} else if okM {
			cv.SafetyReco = recoView(metricsRec)
			cv.Rationale = metricsRec.Rationale
		}

		if req, ok := resMap[pc.Name]; ok {
			cv.CurrentCPU.Req = formatQuantity(req.Requests.Cpu())
			cv.CurrentCPU.Lim = formatQuantity(req.Limits.Cpu())
			cv.CurrentMem.Req = formatQuantity(req.Requests.Memory())
			cv.CurrentMem.Lim = formatQuantity(req.Limits.Memory())
		} else {
			cv.CurrentCPU.Req, cv.CurrentCPU.Lim = dashPlaceholder, dashPlaceholder
			cv.CurrentMem.Req, cv.CurrentMem.Lim = dashPlaceholder, dashPlaceholder
		}

		// Throttle not persisted on CR; omit (null in JSON).

		pv.Containers = append(pv.Containers, cv)
	}

	return pv, nil
}

// BuildProfilesResponse lists all WorkloadProfiles and enriches each with ML state.
func BuildProfilesResponse(ctx context.Context, c client.Client) (*ProfilesResponse, error) {
	var list autosizev1.WorkloadProfileList
	if err := c.List(ctx, &list); err != nil {
		return nil, err
	}

	repo := mlstate.NewConfigMapRepository(c)
	out := &ProfilesResponse{Profiles: make([]ProfileView, 0, len(list.Items))}

	for i := range list.Items {
		wp := &list.Items[i]
		ml, err := repo.Load(ctx, wp)
		if err != nil {
			return nil, fmt.Errorf("load ml state for %s/%s: %w", wp.Namespace, wp.Name, err)
		}
		pv, err := BuildProfileView(ctx, c, wp, ml)
		if err != nil {
			return nil, fmt.Errorf("build view for %s/%s: %w", wp.Namespace, wp.Name, err)
		}
		out.Profiles = append(out.Profiles, *pv)
	}

	return out, nil
}
