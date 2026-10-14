package validator

import (
	stderrors "errors"
	"testing"

	bizerrors "github.com/yi-nology/ekit-core/pkg/errors"
)

// TestValidationError_Error_Empty covers the no-errors path:
// returns the default "validation failed" message.
func TestValidationError_Error_Empty(t *testing.T) {
	ve := &ValidationError{}
	if got := ve.Error(); got != "validation failed" {
		t.Errorf("expected 'validation failed', got %q", got)
	}
}

// TestValidationError_Error_Single covers the single-error path:
// returns that error's message.
func TestValidationError_Error_Single(t *testing.T) {
	ve := &ValidationError{errors: []FieldError{{Field: "name", Message: "name required"}}}
	if got := ve.Error(); got != "name required" {
		t.Errorf("expected 'name required', got %q", got)
	}
}

// TestValidationError_Unwrap verifies the Unwrap method returns a
// *BizError with code 400. This is what makes handler.ErrorRespBiz
// map the validation error to HTTP 400.
func TestValidationError_Unwrap(t *testing.T) {
	ve := &ValidationError{errors: []FieldError{{Field: "name", Message: "name required"}}}
	unwrapped := ve.Unwrap()

	var be *bizerrors.BizError
	if !stderrors.As(unwrapped, &be) {
		t.Fatalf("expected *BizError from Unwrap, got %T", unwrapped)
	}
	if be.BizCode != 400 {
		t.Errorf("expected BizCode=400, got %d", be.BizCode)
	}
	if be.HTTPCode != 400 {
		t.Errorf("expected HTTPCode=400, got %d", be.HTTPCode)
	}
}

// TestValidationError_Unwrap_Nested verifies errors.As walks
// through the ValidationError → BizError chain.
func TestValidationError_Unwrap_Nested(t *testing.T) {
	ve := &ValidationError{errors: []FieldError{{Field: "x", Message: "bad"}}}
	var be *bizerrors.BizError
	if !stderrors.As(ve, &be) {
		t.Fatal("expected errors.As to find *BizError through Unwrap chain")
	}
	if be.BizCode != 400 {
		t.Errorf("expected BizCode=400, got %d", be.BizCode)
	}
}

// TestAsValidationError_Nil covers the nil-error path.
func TestAsValidationError_Nil(t *testing.T) {
	if got, ok := AsValidationError(nil); ok || got != nil {
		t.Errorf("expected (nil, false), got (%v, %v)", got, ok)
	}
}

// TestAsValidationError_WrongType covers the wrong-type path.
func TestAsValidationError_WrongType(t *testing.T) {
	if got, ok := AsValidationError(stderrors.New("plain")); ok || got != nil {
		t.Errorf("expected (nil, false) for non-ValidationError, got (%v, %v)", got, ok)
	}
}

// TestIsValidationError covers the convenience wrapper.
func TestIsValidationError_Extended(t *testing.T) {
	ve := &ValidationError{errors: []FieldError{{Field: "x"}}}
	if !IsValidationError(ve) {
		t.Error("expected IsValidationError=true for *ValidationError")
	}
	if IsValidationError(stderrors.New("plain")) {
		t.Error("expected IsValidationError=false for plain error")
	}
	if IsValidationError(nil) {
		t.Error("expected IsValidationError=false for nil")
	}
}

// TestValidationError_Map_Direct covers the field→message map
// via direct struct construction (vs the existing TestValidationError_Map
// in validator_test.go which uses Validate()).
func TestValidationError_Map_Direct(t *testing.T) {
	ve := &ValidationError{errors: []FieldError{
		{Field: "name", Message: "required"},
		{Field: "age", Message: "must be > 0"},
	}}
	m := ve.Map()
	if m["name"] != "required" {
		t.Errorf("expected name=required, got %q", m["name"])
	}
	if m["age"] != "must be > 0" {
		t.Errorf("expected age=must be > 0, got %q", m["age"])
	}
	if len(m) != 2 {
		t.Errorf("expected 2 entries, got %d", len(m))
	}
}

// TestValidationError_Len covers the Len accessor.
func TestValidationError_Len(t *testing.T) {
	ve := &ValidationError{errors: []FieldError{{}, {}, {}}}
	if got := ve.Len(); got != 3 {
		t.Errorf("expected 3, got %d", got)
	}
	empty := &ValidationError{}
	if got := empty.Len(); got != 0 {
		t.Errorf("expected 0 for empty, got %d", got)
	}
}

// TestValidationError_String_Direct verifies the String() format
// includes all fields with their tags (vs the existing
// TestValidationError_String in validator_test.go which uses
// Validate() and only asserts non-empty).
func TestValidationError_String_Direct(t *testing.T) {
	ve := &ValidationError{errors: []FieldError{
		{Field: "name", Tag: "required"},
		{Field: "age", Tag: "gt"},
	}}
	got := ve.String()
	if want := "validation failed: name (required); age (gt)"; got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

// TestNewFieldValidationError verifies the constructor stores the
// errors slice.
func TestNewFieldValidationError(t *testing.T) {
	fieldErrs := []FieldError{{Field: "x", Message: "y"}}
	ve := NewFieldValidationError(fieldErrs)
	if ve == nil {
		t.Fatal("expected non-nil ValidationError")
	}
	if ve.Len() != 1 {
		t.Errorf("expected 1 error, got %d", ve.Len())
	}
}
