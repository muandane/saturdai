package defaults

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
)

func TestParseGlobalDefaultsConfigMap(t *testing.T) {
	t.Parallel()
	gd, err := ParseGlobalDefaultsConfigMap(map[string]string{
		keyCPURequest:    "100m",
		keyCPULimit:      "500m",
		keyMemoryRequest: "128Mi",
		keyMemoryLimit:   "512Mi",
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if gd.CPURequest.Cmp(resource.MustParse("100m")) != 0 {
		t.Fatalf("cpuRequest: got %v", gd.CPURequest)
	}
	if gd.MemoryLimit.Cmp(resource.MustParse("512Mi")) != 0 {
		t.Fatalf("memoryLimit: got %v", gd.MemoryLimit)
	}
}

func TestParseGlobalDefaultsConfigMapMissingKey(t *testing.T) {
	t.Parallel()
	_, err := ParseGlobalDefaultsConfigMap(map[string]string{
		keyCPURequest: "100m",
	})
	if err == nil {
		t.Fatal("expected error for missing keys")
	}
}

func TestParseGlobalDefaultsConfigMapBadQuantity(t *testing.T) {
	t.Parallel()
	_, err := ParseGlobalDefaultsConfigMap(map[string]string{
		keyCPURequest:    "not-a-quantity",
		keyCPULimit:      "500m",
		keyMemoryRequest: "128Mi",
		keyMemoryLimit:   "512Mi",
	})
	if err == nil {
		t.Fatal("expected error for bad quantity")
	}
}
