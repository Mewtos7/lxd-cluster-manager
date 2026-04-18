package api_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Mewtos7/lx-container-weaver/internal/api"
	"golang.org/x/crypto/bcrypt"
)

// validKey is the raw key used to exercise successful authentication paths.
const validKey = "test-api-key-valid"

// mustHashKey returns a bcrypt hash of key using the minimum cost; it fails
// the test immediately if hashing fails.
func mustHashKey(t *testing.T, key string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(key), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt.GenerateFromPassword: %v", err)
	}
	return string(hash)
}

func TestMiddleware_MissingHeader_Returns401(t *testing.T) {
	hash := mustHashKey(t, validKey)
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	wrapped := api.RequireAPIKey(slog.Default(), []string{hash}, inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status: want 401, got %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type: want application/json, got %s", ct)
	}
	wwa := rec.Header().Get("WWW-Authenticate")
	if wwa == "" {
		t.Error("WWW-Authenticate header must be present on 401 responses")
	}
}

func TestMiddleware_MalformedHeader_Returns401(t *testing.T) {
	hash := mustHashKey(t, validKey)
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	wrapped := api.RequireAPIKey(slog.Default(), []string{hash}, inner)

	cases := []string{
		"",
		"Basic dXNlcjpwYXNz",
		"Bearer",
		"Bearer ",
		"bearer-without-space",
	}
	for _, hdr := range cases {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if hdr != "" {
			req.Header.Set("Authorization", hdr)
		}
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("header %q: want 401, got %d", hdr, rec.Code)
		}
	}
}

func TestMiddleware_InvalidKey_Returns401(t *testing.T) {
	hash := mustHashKey(t, validKey)
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	wrapped := api.RequireAPIKey(slog.Default(), []string{hash}, inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer not-the-right-key")
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status: want 401, got %d", rec.Code)
	}
}

func TestMiddleware_ValidKey_PassesThrough(t *testing.T) {
	hash := mustHashKey(t, validKey)
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	wrapped := api.RequireAPIKey(slog.Default(), []string{hash}, inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+validKey)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status: want 204, got %d", rec.Code)
	}
}

func TestMiddleware_MultipleKeys_AnyValid(t *testing.T) {
	hash1 := mustHashKey(t, "key-one")
	hash2 := mustHashKey(t, "key-two")
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	wrapped := api.RequireAPIKey(slog.Default(), []string{hash1, hash2}, inner)

	for _, key := range []string{"key-one", "key-two"} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+key)
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("key %q: want 200, got %d", key, rec.Code)
		}
	}
}

func TestMiddleware_NoKeys_AlwaysUnauthorized(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	wrapped := api.RequireAPIKey(slog.Default(), nil, inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer any-key")
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status: want 401, got %d", rec.Code)
	}
}

func TestHealthEndpoint_NoAuthRequired(t *testing.T) {
	// Health remains open even with API keys configured.
	hash := mustHashKey(t, validKey)
	srv := api.New(":0", slog.Default(), []string{hash})

	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	// No Authorization header.
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("health without auth: want 200, got %d", rec.Code)
	}
}
