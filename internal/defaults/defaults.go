// Package defaults applies in-process defaults for WorkloadProfile spec (LLD-120 complement).
package defaults

import (
	autosizev1 "github.com/muandane/saturdai/api/v1"
)

// EffectiveMode returns spec.mode or "balanced".
func EffectiveMode(spec autosizev1.WorkloadProfileSpec) string {
	if spec.Mode != "" {
		return spec.Mode
	}
	return "balanced"
}

// Cooldown returns spec cooldown or 15 minutes.
func Cooldown(spec autosizev1.WorkloadProfileSpec) int32 {
	if spec.CooldownMinutes != nil {
		return *spec.CooldownMinutes
	}
	return 15
}

// CollectionInterval returns spec interval or 30 seconds.
func CollectionInterval(spec autosizev1.WorkloadProfileSpec) int32 {
	if spec.CollectionIntervalSeconds != nil {
		return *spec.CollectionIntervalSeconds
	}
	return 30
}
