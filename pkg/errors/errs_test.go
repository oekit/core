package errors

import (
	"errors"
	"testing"
)

func TestAppend_NilLeft(t *testing.T) {
	err := Append(nil, errors.New("right"))
	if err == nil {
		t.Error("expected non-nil")
	}
	if err.Error() != "right" {
		t.Errorf("expected 'right', got %q", err.Error())
	}
}

func TestAppend_NilRight(t *testing.T) {
	err := Append(errors.New("left"), nil)
	if err == nil {
		t.Error("expected non-nil")
	}
	if err.Error() != "left" {
		t.Errorf("expected 'left', got %q", err.Error())
	}
}

func TestAppend_BothNil(t *testing.T) {
	err := Append(nil, nil)
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestAppend_TwoErrors(t *testing.T) {
	err := Append(errors.New("a"), errors.New("b"))
	if err == nil {
		t.Fatal("expected non-nil")
	}
	if err.Error() != "a; b" {
		t.Errorf("expected 'a; b', got %q", err.Error())
	}
}

func TestAppend_Multiple(t *testing.T) {
	var err error
	err = Append(err, errors.New("a"))
	err = Append(err, errors.New("b"))
	err = Append(err, errors.New("c"))

	if err.Error() != "a; b; c" {
		t.Errorf("expected 'a; b; c', got %q", err.Error())
	}
}

func TestMultiError_Is(t *testing.T) {
	target := errors.New("target")
	err := Append(errors.New("other"), target)

	if !errors.Is(err, target) {
		t.Error("expected Is to find target")
	}
}

func TestMultiError_Is_Nil(t *testing.T) {
	var err *MultiError
	if errors.Is(err, errors.New("any")) {
		t.Error("expected false for nil MultiError")
	}
}

func TestMultiError_Errors(t *testing.T) {
	err := Append(errors.New("a"), errors.New("b"))
	merr, ok := err.(*MultiError)
	if !ok {
		t.Fatal("expected *MultiError")
	}
	errs := merr.Errors()
	if len(errs) != 2 {
		t.Errorf("expected 2 errors, got %d", len(errs))
	}
}

func TestMultiError_Error_Nil(t *testing.T) {
	var err *MultiError
	if err.Error() != "" {
		t.Errorf("expected empty string, got %q", err.Error())
	}
}
