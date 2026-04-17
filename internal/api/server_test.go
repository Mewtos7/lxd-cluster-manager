package api_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Mewtos7/lx-container-weaver/internal/api"
)

func TestHealthEndpoint(t *testing.T) {
	srv := api.New(":0", slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	rec := httptest.NewRecorder()

	// Exercise the handler via the exported test helper.
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: want %d, got %d", http.StatusOK, rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type: want application/json, got %s", ct)
	}

	body := rec.Body.String()
	if body != `{"status":"ok"}` {
		t.Errorf("body: want %q, got %q", `{"status":"ok"}`, body)
	}
}
