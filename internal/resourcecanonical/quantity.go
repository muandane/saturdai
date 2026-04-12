// Package resourcecanonical normalizes k8s.io/apimachinery/pkg/api/resource.Quantity values
// for stable, human-readable serialization in CR status (e.g. Ki/Mi/m suffixes vs raw byte integers).
package resourcecanonical

import (
	autosizev1 "github.com/muandane/saturdai/api/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// Canonicalize round-trips a quantity through its canonical string form.
// Semantics are preserved (Cmp unchanged); YAML/JSON output is typically more readable.
func Canonicalize(q resource.Quantity) (resource.Quantity, error) {
	return resource.ParseQuantity(q.String())
}

// CanonicalizeRecommendation applies Canonicalize to all resource fields on r.
func CanonicalizeRecommendation(r autosizev1.Recommendation) (autosizev1.Recommendation, error) {
	var err error
	r.CPURequest, err = Canonicalize(r.CPURequest)
	if err != nil {
		return autosizev1.Recommendation{}, err
	}
	r.CPULimit, err = Canonicalize(r.CPULimit)
	if err != nil {
		return autosizev1.Recommendation{}, err
	}
	r.MemoryRequest, err = Canonicalize(r.MemoryRequest)
	if err != nil {
		return autosizev1.Recommendation{}, err
	}
	r.MemoryLimit, err = Canonicalize(r.MemoryLimit)
	if err != nil {
		return autosizev1.Recommendation{}, err
	}
	return r, nil
}

// CanonicalizeRecommendations maps CanonicalizeRecommendation over recs.
func CanonicalizeRecommendations(recs []autosizev1.Recommendation) ([]autosizev1.Recommendation, error) {
	if len(recs) == 0 {
		return recs, nil
	}
	out := make([]autosizev1.Recommendation, len(recs))
	for i := range recs {
		r, err := CanonicalizeRecommendation(recs[i])
		if err != nil {
			return nil, err
		}
		out[i] = r
	}
	return out, nil
}
