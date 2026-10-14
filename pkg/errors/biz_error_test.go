package errors

import (
	stderrors "errors"
	"fmt"
	"testing"
)

func TestBizError_Error(t *testing.T) {
	err := NewBizError(400, "bad request")
	expected := "biz_code:400 bad request"
	if err.Error() != expected {
		t.Errorf("expected '%s', got '%s'", expected, err.Error())
	}

	// with cause
	cause := stderrors.New("original error")
	err2 := WrapBizError(500, "internal error", cause)
	expected2 := "biz_code:500 internal error: original error"
	if err2.Error() != expected2 {
		t.Errorf("expected '%s', got '%s'", expected2, err2.Error())
	}
}

func TestBizError_Unwrap(t *testing.T) {
	cause := stderrors.New("root cause")
	err := WrapBizError(500, "wrapped", cause)

	if !stderrors.Is(err, cause) {
		t.Error("expected errors.Is to match cause")
	}
}

func TestBizError_String(t *testing.T) {
	err := NewBizErrorWithStatus(404, 404, "not found")
	expected := "biz_code:404 http_code:404 msg:not found"
	if err.String() != expected {
		t.Errorf("expected '%s', got '%s'", expected, err.String())
	}
}

func TestNewNotFoundError(t *testing.T) {
	err := NewNotFoundError("user not found")
	if err.BizCode != 404 || err.HTTPCode != 404 {
		t.Errorf("expected biz/http 404, got biz=%d http=%d", err.BizCode, err.HTTPCode)
	}
	if err.Message != "user not found" {
		t.Errorf("expected 'user not found', got '%s'", err.Message)
	}
}

func TestNewBadRequestError(t *testing.T) {
	err := NewBadRequestError("invalid param")
	if err.BizCode != 400 || err.HTTPCode != 400 {
		t.Errorf("expected biz/http 400, got biz=%d http=%d", err.BizCode, err.HTTPCode)
	}
}

func TestNewUnauthorizedError(t *testing.T) {
	err := NewUnauthorizedError("token expired")
	if err.BizCode != 401 || err.HTTPCode != 401 {
		t.Errorf("expected biz/http 401, got biz=%d http=%d", err.BizCode, err.HTTPCode)
	}
}

func TestNewForbiddenError(t *testing.T) {
	err := NewForbiddenError("no permission")
	if err.BizCode != 403 || err.HTTPCode != 403 {
		t.Errorf("expected biz/http 403, got biz=%d http=%d", err.BizCode, err.HTTPCode)
	}
}

func TestNewConflictError(t *testing.T) {
	err := NewConflictError("duplicate entry")
	if err.BizCode != 409 || err.HTTPCode != 409 {
		t.Errorf("expected biz/http 409, got biz=%d http=%d", err.BizCode, err.HTTPCode)
	}
}

// TestNewBizErrorWithStatus 验证显式两码分离构造（v0.17.0 新增）。
func TestNewBizErrorWithStatus(t *testing.T) {
	err := NewBizErrorWithStatus(1001, 402, "insufficient balance")
	if err.BizCode != 1001 || err.HTTPCode != 402 {
		t.Errorf("expected biz=1001 http=402, got biz=%d http=%d", err.BizCode, err.HTTPCode)
	}
	// NewBizError 仅设 BizCode，HTTPCode 应为 0（走 mapper 回退）
	basic := NewBizError(1001, "msg")
	if basic.BizCode != 1001 || basic.HTTPCode != 0 {
		t.Errorf("expected biz=1001 http=0, got biz=%d http=%d", basic.BizCode, basic.HTTPCode)
	}
}

func TestIsBizError(t *testing.T) {
	bizErr := NewBizError(400, "bad")
	if !IsBizError(bizErr) {
		t.Error("expected IsBizError to return true for BizError")
	}

	normalErr := stderrors.New("normal error")
	if IsBizError(normalErr) {
		t.Error("expected IsBizError to return false for non-BizError")
	}

	// wrapped error
	wrapped := fmt.Errorf("context: %w", bizErr)
	if !IsBizError(wrapped) {
		t.Error("expected IsBizError to return true for wrapped BizError")
	}
}

func TestAsBizError(t *testing.T) {
	bizErr := NewBizError(400, "bad request")

	extracted, ok := AsBizError(bizErr)
	if !ok {
		t.Fatal("expected AsBizError to return true")
	}
	if extracted.BizCode != 400 {
		t.Errorf("expected biz_code 400, got %d", extracted.BizCode)
	}

	// non-BizError
	normalErr := stderrors.New("normal")
	_, ok = AsBizError(normalErr)
	if ok {
		t.Error("expected AsBizError to return false for non-BizError")
	}

	// wrapped
	wrapped := fmt.Errorf("wrap: %w", bizErr)
	extracted, ok = AsBizError(wrapped)
	if !ok {
		t.Fatal("expected AsBizError to return true for wrapped")
	}
	if extracted.BizCode != 400 {
		t.Errorf("expected biz_code 400, got %d", extracted.BizCode)
	}
}
