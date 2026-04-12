package resourcecanonical

import (
	"testing"

	autosizev1 "github.com/muandane/saturdai/api/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestCanonicalize_roundTripEqual(t *testing.T) {
	t.Parallel()
	q := resource.MustParse("1577165")
	got, err := Canonicalize(q)
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}
	if got.Cmp(q) != 0 {
		t.Fatalf("Cmp: got %q want semantically %q", got.String(), q.String())
	}
}

func TestCanonicalize_memoryBytesSemanticPreserved(t *testing.T) {
	t.Parallel()
	raw := resource.NewQuantity(1577165, resource.BinarySI)
	can, err := Canonicalize(*raw)
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}
	if can.Cmp(*raw) != 0 {
		t.Fatalf("semantic mismatch: %q vs %q", can.String(), raw.String())
	}
	again, err := Canonicalize(can)
	if err != nil {
		t.Fatalf("second Canonicalize: %v", err)
	}
	if again.String() != can.String() {
		t.Fatalf("expected idempotent String(), got %q then %q", can.String(), again.String())
	}
}

func TestCanonicalizeRecommendations_empty(t *testing.T) {
	t.Parallel()
	out, err := CanonicalizeRecommendations(nil)
	if err != nil {
		t.Fatal(err)
	}
	if out != nil {
		t.Fatalf("want nil slice for nil input, got %#v", out)
	}
	out2, err := CanonicalizeRecommendations([]autosizev1.Recommendation{})
	if err != nil {
		t.Fatal(err)
	}
	if out2 == nil || len(out2) != 0 {
		t.Fatalf("want empty non-nil slice, got %#v", out2)
	}
}

func TestCanonicalizeRecommendation_allFields(t *testing.T) {
	t.Parallel()
	r := autosizev1.Recommendation{
		ContainerName: "app",
		CPURequest:    *resource.NewMilliQuantity(25, resource.DecimalSI),
		CPULimit:      *resource.NewMilliQuantity(49, resource.DecimalSI),
		MemoryRequest: *resource.NewQuantity(31457280, resource.BinarySI),
		MemoryLimit:   *resource.NewQuantity(93323264, resource.BinarySI),
		Rationale:     "test",
	}
	got, err := CanonicalizeRecommendation(r)
	if err != nil {
		t.Fatal(err)
	}
	if got.CPURequest.Cmp(r.CPURequest) != 0 || got.CPULimit.Cmp(r.CPULimit) != 0 {
		t.Fatalf("cpu mismatch")
	}
	if got.MemoryRequest.Cmp(r.MemoryRequest) != 0 || got.MemoryLimit.Cmp(r.MemoryLimit) != 0 {
		t.Fatalf("memory mismatch")
	}
	if got.Rationale != r.Rationale || got.ContainerName != r.ContainerName {
		t.Fatalf("metadata changed")
	}
}
