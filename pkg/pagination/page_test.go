package pagination

import (
	"testing"
)

func TestNewPageRequest(t *testing.T) {
	req := NewPageRequest(2, 50)
	if req.Page != 2 {
		t.Errorf("expected Page=2, got %d", req.Page)
	}
	if req.PageSize != 50 {
		t.Errorf("expected PageSize=50, got %d", req.PageSize)
	}
}

func TestNormalize_Defaults(t *testing.T) {
	req := NewPageRequest(0, 0)
	req.Normalize()
	if req.Page != DefaultPage {
		t.Errorf("expected Page=%d, got %d", DefaultPage, req.Page)
	}
	if req.PageSize != DefaultPageSize {
		t.Errorf("expected PageSize=%d, got %d", DefaultPageSize, req.PageSize)
	}
}

func TestNormalize_MaxPageSize(t *testing.T) {
	req := NewPageRequest(1, 1000)
	req.Normalize()
	if req.PageSize != MaxPageSize {
		t.Errorf("expected PageSize=%d (max), got %d", MaxPageSize, req.PageSize)
	}
}

func TestNormalize_NegativeValues(t *testing.T) {
	req := NewPageRequest(-1, -5)
	req.Normalize()
	if req.Page != DefaultPage {
		t.Errorf("expected Page=%d, got %d", DefaultPage, req.Page)
	}
	if req.PageSize != DefaultPageSize {
		t.Errorf("expected PageSize=%d, got %d", DefaultPageSize, req.PageSize)
	}
}

func TestOffset(t *testing.T) {
	req := NewPageRequest(3, 20)
	if req.Offset() != 40 {
		t.Errorf("expected Offset=40, got %d", req.Offset())
	}
}

func TestLimit(t *testing.T) {
	req := NewPageRequest(1, 25)
	if req.Limit() != 25 {
		t.Errorf("expected Limit=25, got %d", req.Limit())
	}
}

func TestConstants(t *testing.T) {
	if DefaultPage != 1 {
		t.Errorf("expected DefaultPage=1, got %d", DefaultPage)
	}
	if DefaultPageSize != 20 {
		t.Errorf("expected DefaultPageSize=20, got %d", DefaultPageSize)
	}
	if MaxPageSize != 500 {
		t.Errorf("expected MaxPageSize=500, got %d", MaxPageSize)
	}
}
