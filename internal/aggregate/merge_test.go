package aggregate

import (
	"testing"

	"github.com/DataDog/sketches-go/ddsketch"
)

func TestMergeSketchesFromBase64_empty(t *testing.T) {
	sk, err := MergeSketchesFromBase64(nil)
	if err != nil {
		t.Fatal(err)
	}
	if sk == nil || !sk.IsEmpty() {
		t.Fatalf("expected empty default sketch")
	}
}

func TestMergeSketchesFromBase64_roundTrip(t *testing.T) {
	a, err := ddsketch.NewDefaultDDSketch(0.01)
	if err != nil {
		t.Fatal(err)
	}
	_ = a.Add(100)
	_ = a.Add(200)
	sa, err := SketchToBase64(a)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ddsketch.NewDefaultDDSketch(0.01)
	if err != nil {
		t.Fatal(err)
	}
	_ = b.Add(300)
	sb, err := SketchToBase64(b)
	if err != nil {
		t.Fatal(err)
	}
	merged, err := MergeSketchesFromBase64([]string{sa, sb})
	if err != nil {
		t.Fatal(err)
	}
	if merged.IsEmpty() || merged.GetCount() < 2 {
		t.Fatalf("expected merged count >= 2, got %v", merged.GetCount())
	}
}
