package api

import (
	"net/http/httptest"
	"testing"
)

func TestParsePagination_Defaults(t *testing.T) {
	r := httptest.NewRequest("GET", "/v1/runs", nil)
	p := ParsePagination(r)
	if p.Limit != 100 {
		t.Errorf("expected default limit 100, got %d", p.Limit)
	}
	if p.Offset != 0 {
		t.Errorf("expected default offset 0, got %d", p.Offset)
	}
}

func TestParsePagination_Custom(t *testing.T) {
	r := httptest.NewRequest("GET", "/v1/runs?limit=10&offset=20", nil)
	p := ParsePagination(r)
	if p.Limit != 10 {
		t.Errorf("expected limit 10, got %d", p.Limit)
	}
	if p.Offset != 20 {
		t.Errorf("expected offset 20, got %d", p.Offset)
	}
}

func TestParsePagination_InvalidValues(t *testing.T) {
	r := httptest.NewRequest("GET", "/v1/runs?limit=abc&offset=-5", nil)
	p := ParsePagination(r)
	if p.Limit != 100 {
		t.Errorf("expected default limit 100 for invalid, got %d", p.Limit)
	}
	if p.Offset != 0 {
		t.Errorf("expected default offset 0 for invalid, got %d", p.Offset)
	}
}

func TestParsePagination_CapsAt1000(t *testing.T) {
	r := httptest.NewRequest("GET", "/v1/runs?limit=5000", nil)
	p := ParsePagination(r)
	if p.Limit != 100 {
		t.Errorf("expected capped limit 100, got %d", p.Limit)
	}
}

func TestBuildLinks(t *testing.T) {
	r := httptest.NewRequest("GET", "/v1/runs?limit=10&offset=0", nil)
	links := BuildLinks(r, 25, 10, 0)
	if links.Next == "" {
		t.Error("expected next link")
	}
	if links.Prev != "" {
		t.Error("expected no prev link at offset 0")
	}
	if links.Self == "" {
		t.Error("expected self link")
	}
}

func TestBuildLinks_Middle(t *testing.T) {
	r := httptest.NewRequest("GET", "/v1/runs?limit=10&offset=10", nil)
	links := BuildLinks(r, 25, 10, 10)
	if links.Next == "" {
		t.Error("expected next link")
	}
	if links.Prev == "" {
		t.Error("expected prev link")
	}
}

func TestBuildLinks_LastPage(t *testing.T) {
	r := httptest.NewRequest("GET", "/v1/runs?limit=10&offset=20", nil)
	links := BuildLinks(r, 25, 10, 20)
	if links.Next != "" {
		t.Error("expected no next link on last page")
	}
	if links.Prev == "" {
		t.Error("expected prev link")
	}
}
