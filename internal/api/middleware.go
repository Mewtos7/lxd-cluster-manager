package api

import (
	"log/slog"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

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
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("WWW-Authenticate", `Bearer realm="lx-container-weaver"`)
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"code":"unauthorized","message":"valid API key required"}`))
}
