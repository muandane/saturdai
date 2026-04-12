package aggregate

import (
	"fmt"

	"github.com/DataDog/sketches-go/ddsketch"
)

// MergeSketchesFromBase64 decodes non-empty base64 sketches and merges them with MergeWith.
// Returns a new default sketch if none were valid and non-empty.
func MergeSketchesFromBase64(encoded []string) (*ddsketch.DDSketch, error) {
	var out *ddsketch.DDSketch
	for _, e := range encoded {
		if e == "" {
			continue
		}
		sk, err := SketchFromBase64(e)
		if err != nil {
			continue
		}
		if sk == nil || sk.IsEmpty() {
			continue
		}
		if out == nil {
			c := sk.Copy()
			out = c
			continue
		}
		if err := out.MergeWith(sk); err != nil {
			return nil, fmt.Errorf("merge sketch: %w", err)
		}
	}
	if out == nil {
		return ddsketch.NewDefaultDDSketch(relativeAccuracy)
	}
	return out, nil
}
