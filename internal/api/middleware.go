package api

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// contextKey is an unexported type for context keys in this package, preventing
// key collisions with context keys from other packages.
type contextKey int

const requestIDKey contextKey = iota

// RequestIDFromContext returns the request ID stored in ctx by the RequestID
// middleware. It returns an empty string if no request ID is present.
func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

// responseWriter wraps http.ResponseWriter to capture the HTTP status code
// written by a handler so that middleware can observe the final response status.
type responseWriter struct {
	http.ResponseWriter
	status  int
	written bool
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{ResponseWriter: w, status: http.StatusOK}
}

func (rw *responseWriter) WriteHeader(status int) {
	if !rw.written {
		rw.status = status
		rw.written = true
		rw.ResponseWriter.WriteHeader(status)
	}
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.written {
		rw.status = http.StatusOK
		rw.written = true
	}
	return rw.ResponseWriter.Write(b)
}

// newRequestID generates a random UUID v4 string for use as a request
// identifier. Falls back to a fixed placeholder if the random source fails.
func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "00000000-0000-4000-8000-000000000000"
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant bits
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// RequestID is middleware that reads the X-Request-ID header from the incoming
// request, or generates a new UUID v4 if absent. The ID is stored in the
// request context (retrievable via RequestIDFromContext) and echoed back in the
// X-Request-ID response header.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = newRequestID()
		}
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestLogger returns middleware that emits a structured log entry for every
// handled request, including method, path, status code, duration, request ID,
// and remote address. It relies on RequestID middleware being applied upstream
// to populate the request ID in context.
func RequestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := newResponseWriter(w)
		next.ServeHTTP(rw, r)
		logger.Info("http request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", rw.status),
			slog.Duration("duration", time.Since(start)),
			slog.String("request_id", RequestIDFromContext(r.Context())),
			slog.String("remote_addr", r.RemoteAddr),
		)
	})
}

// Recoverer returns middleware that catches any panic raised by a downstream
// handler, logs the error, and returns a 500 Internal Server Error with a
// structured JSON body, preventing the process from crashing.
func Recoverer(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.Error("http handler panic",
					slog.Any("panic", rec),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.String("request_id", RequestIDFromContext(r.Context())),
				)
				writeInternalError(w)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// writeError writes a JSON error response with the given HTTP status code.
// The body format conforms to the API error contract: {"code":"...","message":"..."}.
func writeError(w http.ResponseWriter, statusCode int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	b, _ := json.Marshal(struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{Code: code, Message: message})
	_, _ = w.Write(b)
}

// writeInternalError writes a 500 Internal Server Error JSON response.
func writeInternalError(w http.ResponseWriter) {
	writeError(w, http.StatusInternalServerError, "internal_error", "an unexpected error occurred")
}

// RequireAPIKey returns an http.Handler middleware that enforces Bearer token
// authentication on every request it wraps. The provided hashes must be
// bcrypt-hashed values of the raw API keys issued to clients (ADR-003).
//
// Requests without an Authorization header, with a non-Bearer scheme, or
// whose token does not match any stored hash are rejected with 401 Unauthorized
// and a JSON error body consistent with the API contract.
//
// This function is exported so that individual route groups can apply it
// selectively. The health endpoint (/v1/health) is intentionally excluded from
// this middleware and must remain reachable by unauthenticated liveness probes.
func RequireAPIKey(logger *slog.Logger, hashes []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			logger.Warn("auth: missing or malformed Authorization header",
				slog.String("remote_addr", r.RemoteAddr),
				slog.String("path", r.URL.Path),
			)
			writeUnauthorized(w)
			return
		}

		if !validateAPIKey(token, hashes) {
			logger.Warn("auth: invalid API key",
				slog.String("remote_addr", r.RemoteAddr),
				slog.String("path", r.URL.Path),
			)
			writeUnauthorized(w)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// requireAPIKey is a convenience method on Server that delegates to
// RequireAPIKey with the server's configured hashes and logger.
func (s *Server) requireAPIKey(next http.Handler) http.Handler {
	return RequireAPIKey(s.logger, s.apiKeyHashes, next)
}

// bearerToken extracts the raw token from an "Authorization: Bearer <token>"
// header. It returns the token and true on success, or an empty string and
// false if the header is absent or uses a different scheme.
func bearerToken(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return "", false
	}
	scheme, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "bearer") || token == "" {
		return "", false
	}
	return token, true
}

// validateAPIKey returns true if the provided raw token matches any of the
// stored bcrypt hashes. It iterates over all hashes so that response time
// does not reveal how many hashes are stored.
func validateAPIKey(token string, hashes []string) bool {
	matched := false
	for _, hash := range hashes {
		if bcrypt.CompareHashAndPassword([]byte(hash), []byte(token)) == nil {
			matched = true
			// Continue iterating to avoid short-circuit timing differences.
		}
	}
	return matched
}

// writeUnauthorized writes a 401 Unauthorized response with a JSON body that
// conforms to the API error contract: { "code", "message" }.
func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="lx-container-weaver"`)
	writeError(w, http.StatusUnauthorized, "unauthorized", "valid API key required")
}
