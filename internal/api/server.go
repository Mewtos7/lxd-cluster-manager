// Package api contains the HTTP server and handler wiring for the manager REST
// API. Endpoints are versioned under the /v1/ path prefix as defined in
// ADR-002.
package api

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// Server wraps the standard library HTTP server and owns the route registrations.
// It implements http.Handler so that individual handlers can be exercised in
// unit tests without binding to a real network port.
type Server struct {
	srv          *http.Server
	mux          *http.ServeMux
	handler      http.Handler // mux with base middleware applied
	logger       *slog.Logger
	apiKeyHashes []string
}

// New creates a new Server bound to addr. The provided logger is used for
// request-level diagnostics. apiKeyHashes must be a slice of bcrypt-hashed API
// keys; protected routes require a matching Bearer token in every request.
func New(addr string, logger *slog.Logger, apiKeyHashes []string) *Server {
	mux := http.NewServeMux()

	// Base middleware chain applied to all routes, authenticated or not:
	//   RequestID → RequestLogger → Recoverer → mux
	// RequestID must be outermost so the ID is available to all downstream
	// middleware. RequestLogger wraps Recoverer so that panic-recovered 500
	// responses are captured in the access log.
	handler := RequestID(RequestLogger(logger, Recoverer(logger, mux)))

	s := &Server{
		logger:       logger,
		mux:          mux,
		handler:      handler,
		apiKeyHashes: apiKeyHashes,
	}

	// Unauthenticated routes — reachable by liveness probes without credentials.
	mux.HandleFunc("GET /v1/health", s.handleHealth)

	s.srv = &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
		BaseContext: func(_ net.Listener) context.Context {
			return context.Background()
		},
	}

	return s
}

// ServeHTTP implements http.Handler, delegating to the full middleware chain.
// This allows the server's routes to be exercised in unit tests without binding
// to a real network port, while still exercising request ID, logging, and
// panic recovery middleware.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

// ListenAndServe starts the HTTP server. It blocks until the server is shut
// down or encounters a fatal error. Callers should invoke Shutdown to trigger
// graceful termination.
func (s *Server) ListenAndServe() error {
	s.logger.Info("HTTP server starting", "addr", s.srv.Addr)
	return s.srv.ListenAndServe()
}

// Shutdown gracefully stops the server, waiting up to ctx's deadline for
// in-flight requests to complete.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("HTTP server shutting down")
	return s.srv.Shutdown(ctx)
}

// handleHealth responds to GET /v1/health with a 200 OK. It serves as both a
// liveness probe and a basic startup check confirming that the service is
// reachable.
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
