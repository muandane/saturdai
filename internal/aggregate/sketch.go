package aggregate

import (
	"encoding/base64"
	"errors"

	"github.com/DataDog/sketches-go/ddsketch"
	"github.com/DataDog/sketches-go/ddsketch/pb/sketchpb"
	"google.golang.org/protobuf/proto"
)

const relativeAccuracy = 0.01

// SketchFromBase64 decodes a DDSketch from base64-encoded protobuf. Empty string returns a new sketch.
func SketchFromBase64(encoded string) (*ddsketch.DDSketch, error) {
	if encoded == "" {
		return ddsketch.NewDefaultDDSketch(relativeAccuracy)
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	var pb sketchpb.DDSketch
	if err := proto.Unmarshal(raw, &pb); err != nil {
		return nil, err
	}
	return ddsketch.FromProto(&pb)
}

// SketchToBase64 encodes a DDSketch to base64 protobuf.
func SketchToBase64(s *ddsketch.DDSketch) (string, error) {
	if s == nil {
		var err error
		s, err = ddsketch.NewDefaultDDSketch(relativeAccuracy)
		if err != nil {
			return "", err
		}
	}
	b, err := proto.Marshal(s.ToProto())
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// Quantile returns the value at quantile q in (0,1).
func Quantile(s *ddsketch.DDSketch, q float64) (float64, error) {
	if s == nil || s.IsEmpty() {
		return 0, errors.New("empty sketch")
	}
	return s.GetValueAtQuantile(q)
}
