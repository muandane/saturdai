package aggregate

import (
	"github.com/DataDog/sketches-go/ddsketch"
)

// ResourceSample drives Update with sketch persistence hooks and one usage sample.
type ResourceSample struct {
	Value float64
	// GetSketch returns the persisted base64 sketch (may be empty).
	GetSketch func() string
	// SetSketch persists the encoded sketch.
	SetSketch func(string)
	// GetEMA returns short and long EMA values before this sample.
	GetEMA func() (short, long float64)
	// SetEMA writes EMA after UpdateEMA when Value > 0.
	SetEMA func(short, long float64)
	// OnAddError is called when sketch Add fails (optional; matches prior reconcile logging).
	OnAddError func(err error)
}

func decodeSketch(encoded string) (*ddsketch.DDSketch, error) {
	sk, err := SketchFromBase64(encoded)
	if err != nil {
		// Parity with controller loadSketches: treat corrupt blob as empty sketch.
		return SketchFromBase64("")
	}
	return sk, nil
}

// Update decodes the sketch, always Add(Value), encodes, then updates EMA only when Value > 0.
func Update(s ResourceSample) error {
	val := FiniteOrZero(s.Value)
	sk, err := decodeSketch(s.GetSketch())
	if err != nil {
		return err
	}
	if err := sk.Add(val); err != nil && s.OnAddError != nil {
		s.OnAddError(err)
	}
	encoded, err := SketchToBase64(sk)
	if err != nil {
		return err
	}
	s.SetSketch(encoded)

	if val > 0 {
		short, long := s.GetEMA()
		short, long = UpdateEMA(FiniteOrZero(short), FiniteOrZero(long), val)
		short, long = sanitizeEMA(short, long)
		s.SetEMA(short, long)
	}
	return nil
}

// UpdateSketchOnly appends one sample to a DDSketch without updating EMA (per-node sketches, LLD-300).
func UpdateSketchOnly(s ResourceSample) error {
	val := FiniteOrZero(s.Value)
	sk, err := decodeSketch(s.GetSketch())
	if err != nil {
		return err
	}
	if err := sk.Add(val); err != nil && s.OnAddError != nil {
		s.OnAddError(err)
	}
	encoded, err := SketchToBase64(sk)
	if err != nil {
		return err
	}
	s.SetSketch(encoded)
	return nil
}

func sanitizeEMA(short, long float64) (float64, float64) {
	return FiniteOrZero(short), FiniteOrZero(long)
}
