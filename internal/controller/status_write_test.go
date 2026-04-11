package controller

import (
	"errors"
	"fmt"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestBenignStatusAPIErr(t *testing.T) {
	uidPrecond := errors.New(`Operation cannot be fulfilled: StorageError: invalid object, AdditionalErrorMsg: Precondition failed: UID in precondition: abc, UID in object meta: `)
	notFound := apierrors.NewNotFound(schema.GroupResource{Group: "autosize.saturdai.auto", Resource: "workloadprofiles"}, "x")
	gone := apierrors.NewResourceExpired("gone")
	internalOther := apierrors.NewInternalError(fmt.Errorf("database exploded"))

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"not found", notFound, true},
		{"gone", gone, true},
		{"uid precondition delete race", uidPrecond, true},
		{"internal other", internalOther, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := benignStatusAPIErr(tt.err); got != tt.want {
				t.Errorf("benignStatusAPIErr() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBenignStatusAPIErr_UIDFromStatus(t *testing.T) {
	st := metav1.Status{
		Status:  metav1.StatusFailure,
		Message: `Precondition failed: UID in precondition: x, UID in object meta: `,
		Code:    500,
		Reason:  metav1.StatusReasonInternalError,
	}
	err := apierrors.FromObject(&st)
	if !benignStatusAPIErr(err) {
		t.Error("expected benign for Status-shaped UID precondition message")
	}
}
