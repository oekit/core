package errors

import (
	stderrors "errors"
	"testing"
)

// TestInspect_AllNil covers the all-nil input case: count stays 0,
// no first error idx.
func TestInspect_AllNil(t *testing.T) {
	res := inspect([]error{nil, nil, nil})
	if res.Count != 0 {
		t.Errorf("expected Count=0, got %d", res.Count)
	}
	if res.Capacity != 0 {
		t.Errorf("expected Capacity=0, got %d", res.Capacity)
	}
}

// TestInspect_MixedNil covers the mixed nil + non-nil case: nils
// are skipped, only non-nil errors count.
func TestInspect_MixedNil(t *testing.T) {
	res := inspect([]error{nil, stderrors.New("a"), nil, stderrors.New("b"), nil})
	if res.Count != 2 {
		t.Errorf("expected Count=2, got %d", res.Count)
	}
	if res.Capacity != 2 {
		t.Errorf("expected Capacity=2, got %d", res.Capacity)
	}
	if res.FirstErrorIdx != 1 {
		t.Errorf("expected FirstErrorIdx=1, got %d", res.FirstErrorIdx)
	}
}

// TestInspect_NestedMultiError covers the "contains MultiError" branch.
// A nested MultiError should bump Capacity by len(nested.errors) and
// set ContainsMultiError=true.
func TestInspect_NestedMultiError(t *testing.T) {
	nested := &MultiError{errors: []error{stderrors.New("a"), stderrors.New("b")}}
	outer := []error{stderrors.New("x"), nested, stderrors.New("y")}

	res := inspect(outer)
	if res.Count != 3 {
		t.Errorf("expected Count=3, got %d", res.Count)
	}
	if res.Capacity != 4 { // 1 + 2 nested + 1
		t.Errorf("expected Capacity=4, got %d", res.Capacity)
	}
	if !res.ContainsMultiError {
		t.Error("expected ContainsMultiError=true")
	}
}

// TestFromSlice_AllNil covers the "no non-nil errors" return.
func TestFromSlice_AllNil(t *testing.T) {
	got := fromSlice([]error{nil, nil})
	if got != nil {
		t.Errorf("expected nil for all-nil, got %v", got)
	}
}

// TestFromSlice_SingleError covers the optimization: when only one
// non-nil error is present, return it directly (not wrapped in
// MultiError).
func TestFromSlice_SingleError(t *testing.T) {
	err := stderrors.New("only")
	got := fromSlice([]error{nil, err, nil})
	if got != err {
		t.Errorf("expected the original error, got %v", got)
	}
	// Must NOT be wrapped in MultiError.
	if _, isMerr := got.(*MultiError); isMerr {
		t.Error("expected single error to be returned as-is, not wrapped")
	}
}

// TestFromSlice_AllNonNil_NoMultiError covers the "len==count and no
// nested MultiError" branch: return the slice as a MultiError
// directly (no copy).
func TestFromSlice_AllNonNil_NoMultiError(t *testing.T) {
	errs := []error{stderrors.New("a"), stderrors.New("b"), stderrors.New("c")}
	got := fromSlice(errs)
	merr, ok := got.(*MultiError)
	if !ok {
		t.Fatalf("expected *MultiError, got %T", got)
	}
	if len(merr.errors) != 3 {
		t.Errorf("expected 3 errors, got %d", len(merr.errors))
	}
}

// TestFromSlice_FlattenNested covers the path where a nested
// MultiError is flattened into the outer MultiError.
func TestFromSlice_FlattenNested(t *testing.T) {
	nested := &MultiError{errors: []error{stderrors.New("nested-1"), stderrors.New("nested-2")}}
	outer := []error{stderrors.New("outer-1"), nested, nil, stderrors.New("outer-2")}

	got := fromSlice(outer)
	merr, ok := got.(*MultiError)
	if !ok {
		t.Fatalf("expected *MultiError, got %T", got)
	}
	if len(merr.errors) != 4 {
		t.Errorf("expected 4 errors (2 outer + 2 nested), got %d", len(merr.errors))
	}
}

// TestAppend_LeftIsMultiError covers the copy-on-write fast path
// (right is not MultiError, left is MultiError without copyNeeded).
func TestAppend_LeftIsMultiError(t *testing.T) {
	inner := stderrors.New("inner")
	left := Append(stderrors.New("a"), inner) // creates MultiError
	right := stderrors.New("right")

	got := Append(left, right)
	merr, ok := got.(*MultiError)
	if !ok {
		t.Fatalf("expected *MultiError, got %T", got)
	}
	if len(merr.errors) != 3 {
		t.Errorf("expected 3 errors after Append, got %d", len(merr.errors))
	}
}

// TestAppend_RightIsMultiError covers the fromSlice path (right is
// MultiError, left is plain error).
func TestAppend_RightIsMultiError(t *testing.T) {
	left := stderrors.New("left")
	inner := Append(stderrors.New("a"), stderrors.New("b"))
	// inner is a MultiError of 2 errors.

	got := Append(left, inner)
	merr, ok := got.(*MultiError)
	if !ok {
		t.Fatalf("expected *MultiError, got %T", got)
	}
	if len(merr.errors) != 3 {
		t.Errorf("expected 3 errors, got %d", len(merr.errors))
	}
}

// TestAppend_BothMultiError covers the both-MultiError branch — must
// delegate to fromSlice.
func TestAppend_BothMultiError(t *testing.T) {
	left := Append(stderrors.New("a"), stderrors.New("b"))
	right := Append(stderrors.New("c"), stderrors.New("d"))

	got := Append(left, right)
	merr, ok := got.(*MultiError)
	if !ok {
		t.Fatalf("expected *MultiError, got %T", got)
	}
	if len(merr.errors) != 4 {
		t.Errorf("expected 4 errors, got %d", len(merr.errors))
	}
}

// TestMultiError_Error_Multiple verifies the Error() string with
// multiple non-nil errors uses "; " as separator.
func TestMultiError_Error_Multiple(t *testing.T) {
	merr := &MultiError{errors: []error{stderrors.New("a"), stderrors.New("b"), stderrors.New("c")}}
	if got := merr.Error(); got != "a; b; c" {
		t.Errorf("expected 'a; b; c', got %q", got)
	}
}

// TestMultiError_Errors_ReturnsSlice verifies Errors() returns the
// underlying slice (defensive guard for nil receiver).
func TestMultiError_Errors_ReturnsSlice(t *testing.T) {
	errs := []error{stderrors.New("a"), stderrors.New("b")}
	merr := &MultiError{errors: errs}
	if got := merr.Errors(); len(got) != 2 {
		t.Errorf("expected 2 errors, got %d", len(got))
	}
}

// TestMultiError_Is_NestedErrorMatch verifies Is() walks nested
// MultiErrors via errors.Is.
func TestMultiError_Is_NestedErrorMatch(t *testing.T) {
	target := stderrors.New("target")
	other := stderrors.New("other")
	inner := &MultiError{errors: []error{other, target}}
	outer := &MultiError{errors: []error{inner, stderrors.New("x")}}

	if !stderrors.Is(outer, target) {
		t.Error("expected errors.Is to find 'target' in nested MultiError")
	}
	if stderrors.Is(outer, stderrors.New("missing")) {
		t.Error("expected errors.Is to NOT find 'missing'")
	}
}
