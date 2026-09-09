// Package server implements a small fake signup backend that demonstrates how
// to integrate the Hansestack leak-check client into an authentication flow.
//
// The central rule it illustrates is fail-open: when the leak check cannot
// complete, the signup still succeeds. A supplementary security signal must
// never become a single point of failure.
package server

import (
	"context"
	"log/slog"
	"net/http"
	"sync"

	"github.com/hansestack/hansestack-go/leakcheck"
)

// PasswordChecker is the subset of the leak-check client this package needs.
//
// Depending on an interface rather than the concrete *leakcheck.Client keeps
// the handlers testable without network access.
type PasswordChecker interface {
	CheckPassword(ctx context.Context, password string) (leakcheck.Result, error)
}

// Server holds the demo's dependencies and its in-memory user store.
//
// There is no database on purpose: this exists to demonstrate leak-check
// integration, not persistence.
type Server struct {
	leaks  PasswordChecker
	logger *slog.Logger

	mu    sync.Mutex
	users map[string]string // email -> password hash placeholder
}

// New returns a Server backed by the given checker.
func New(leaks PasswordChecker, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}

	return &Server{
		leaks:  leaks,
		logger: logger,
		users:  make(map[string]string),
	}
}

// Routes returns the demo's HTTP handler with logging middleware applied.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("POST /signup", s.handleSignup)
	mux.HandleFunc("POST /check", s.handleCheck)
	mux.HandleFunc("GET /", s.handleIndex)

	return requestLogger(s.logger, mux)
}
