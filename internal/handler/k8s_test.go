package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestK8sHandler_Health(t *testing.T) {
	h := NewK8sHandler()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/health", nil)

	h.Health(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "OK" {
		t.Errorf("expected body %q, got %q", "OK", w.Body.String())
	}
}

func TestK8sHandler_Ready(t *testing.T) {
	h := NewK8sHandler()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ready", nil)

	h.Ready(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "READY" {
		t.Errorf("expected body %q, got %q", "READY", w.Body.String())
	}
}
