package api_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
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

// ─── RequestID middleware ─────────────────────────────────────────────────────

func TestRequestID_GeneratesHeaderWhenAbsent(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := api.RequestID(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	id := rec.Header().Get("X-Request-ID")
	if id == "" {
		t.Error("X-Request-ID header must be set when the request carries none")
	}
}

func TestRequestID_PropagatesExistingHeader(t *testing.T) {
	const clientID = "my-client-request-id"
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The ID must be available in context.
		if got := api.RequestIDFromContext(r.Context()); got != clientID {
			t.Errorf("context request_id: want %q, got %q", clientID, got)
		}
		w.WriteHeader(http.StatusOK)
	})
	h := api.RequestID(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", clientID)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Request-ID"); got != clientID {
		t.Errorf("X-Request-ID response header: want %q, got %q", clientID, got)
	}
}

func TestRequestID_UniquePerRequest(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := api.RequestID(inner)

	ids := make(map[string]bool)
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		id := rec.Header().Get("X-Request-ID")
		if id == "" {
			t.Fatalf("iteration %d: X-Request-ID must not be empty", i)
		}
		if ids[id] {
			t.Fatalf("duplicate request ID generated: %q", id)
		}
		ids[id] = true
	}
}

// ─── Recoverer middleware ─────────────────────────────────────────────────────

func TestRecoverer_CatchesPanicAndReturns500(t *testing.T) {
	panicking := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("simulated handler panic")
	})
	h := api.Recoverer(slog.Default(), panicking)

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status: want 500, got %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type: want application/json, got %s", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"code"`) || !strings.Contains(body, `"message"`) {
		t.Errorf("body must be a JSON error object, got: %s", body)
	}
}

func TestRecoverer_PassesThroughNormalHandlers(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	h := api.Recoverer(slog.Default(), ok)

	req := httptest.NewRequest(http.MethodGet, "/fine", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status: want 204, got %d", rec.Code)
	}
}

// ─── Full middleware chain via Server ─────────────────────────────────────────

func TestServer_RequestIDHeaderSetOnEveryResponse(t *testing.T) {
	srv := api.New(":0", slog.Default(), nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Header().Get("X-Request-ID") == "" {
		t.Error("X-Request-ID header must be present on all responses")
	}
}

func TestServer_PanicRecoveryViaChain(t *testing.T) {
	// Verify that the full middleware chain (RequestID → RequestLogger →
	// Recoverer) handles panics and returns a structured 500.
	panicking := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("intentional test panic")
	})
	h := api.RequestID(api.RequestLogger(slog.Default(), api.Recoverer(slog.Default(), panicking)))

	req := httptest.NewRequest(http.MethodGet, "/v1/test", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status: want 500, got %d", rec.Code)
	}
	if rec.Header().Get("X-Request-ID") == "" {
		t.Error("X-Request-ID header must be present even when handler panics")
	}
}
