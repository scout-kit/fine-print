package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthz_Alwaysgreen(t *testing.T) {
	h, _ := newTestHandlers(t)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.Healthz(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d", rec.Code)
	}
}

func TestReadyz_DBCheckSucceedsOnFreshStore(t *testing.T) {
	h, _ := newTestHandlers(t)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	h.Readyz(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if resp.Status != "ready" {
		t.Errorf("status: got %q, want ready", resp.Status)
	}
	if resp.Checks["db"] != "ok" {
		t.Errorf("db check: got %q, want ok", resp.Checks["db"])
	}
}
