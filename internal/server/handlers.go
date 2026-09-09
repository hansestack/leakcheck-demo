package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Leak-check outcomes reported back to the caller, so the demo makes the
// fail-open path visible from the outside.
const (
	leakStatusClean       = "clean"       // check ran, password not in the corpus
	leakStatusLeaked      = "leaked"      // check ran, password found
	leakStatusUnavailable = "unavailable" // check could not run; signup proceeded anyway
)

type signupRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type signupResponse struct {
	Status      string `json:"status"`
	LeakCheck   string `json:"leak_check"`
	BreachCount int    `json:"breach_count,omitempty"`
}

type checkResponse struct {
	Leaked  bool `json:"leaked"`
	Count   int  `json:"count"`
	Checked bool `json:"checked"`
}

type errorResponse struct {
	Error     string `json:"error"`
	LeakCheck string `json:"leak_check,omitempty"`
	Count     int    `json:"breach_count,omitempty"`
}

// handleHealth reports the liveness of this service only.
//
// It deliberately does NOT probe the Hansestack API. An outage of a
// supplementary security check must not mark this service unhealthy and get it
// restarted or removed from a load balancer. That is the fail-open principle
// applied at the infrastructure layer.
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleSignup is the reference integration.
//
// Flow:
//  1. Validate the request body.
//  2. Run the leak check, passing the request context.
//  3. On error: log a warning and CONTINUE. Never return 5xx.
//  4. On a confirmed leak: reject with 400, a client-side validation failure.
//  5. Otherwise create the user.
func (s *Server) handleSignup(w http.ResponseWriter, r *http.Request) {
	var req signupRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})

		return
	}

	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "email and password are required"})

		return
	}

	// Pass the request context so the check inherits client cancellation.
	res, err := s.leaks.CheckPassword(r.Context(), req.Password)

	// NOTE ON OBSERVABILITY: with the default fail-open client this error is
	// ALWAYS nil, because the library already swallowed the failure and
	// returned a neutral result. That is the correct production behaviour, but
	// it means the caller cannot tell "checked and clean" apart from "check
	// never ran" — you rely on res.Outcome or the library's own WARN/ERROR
	// logs instead.
	//
	// The leakStatusUnavailable branch below is therefore only reachable with
	// LEAKCHECK_FAIL_CLOSE=true or `serve --simulate-outage`. Both exist so
	// this demo can make the fail-open path visible.
	leakStatus := leakStatusClean
	if err != nil {
		// FAIL OPEN. The leak check is an additional security layer, never a
		// single point of failure. Log it and carry on as if the password were
		// safe. Returning 5xx here would let a Hansestack outage lock every
		// user out of registration.
		s.logger.WarnContext(r.Context(), "leak check unavailable, continuing signup",
			"err", err, "email", req.Email, "outcome", res.Outcome)
		leakStatus = leakStatusUnavailable
	}

	if res.Leaked {
		// A confirmed leak is a validation failure, not a server error.
		s.logger.InfoContext(r.Context(), "signup rejected: leaked password",
			"email", req.Email, "breach_count", res.Count)
		writeJSON(w, http.StatusBadRequest, errorResponse{
			Error:     fmt.Sprintf("this password appeared in %d known data breaches, please choose another", res.Count),
			LeakCheck: leakStatusLeaked,
			Count:     res.Count,
		})

		return
	}

	if err := s.createUser(req.Email, req.Password); err != nil {
		// A genuine server-side failure. This one legitimately returns 409/500.
		writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})

		return
	}

	writeJSON(w, http.StatusCreated, signupResponse{
		Status:    "created",
		LeakCheck: leakStatus,
	})
}

// handleCheck reports on a password without creating a user.
//
// `checked: false` means the lookup could not be performed and the result is a
// neutral default rather than a verified answer.
func (s *Server) handleCheck(w http.ResponseWriter, r *http.Request) {
	var req signupRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})

		return
	}
	if req.Password == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "password is required"})

		return
	}

	res, err := s.leaks.CheckPassword(r.Context(), req.Password)
	if err != nil {
		s.logger.WarnContext(r.Context(), "leak check unavailable", "err", err, "outcome", res.Outcome)
		writeJSON(w, http.StatusOK, checkResponse{Checked: false})

		return
	}

	writeJSON(w, http.StatusOK, checkResponse{Leaked: res.Leaked, Count: res.Count, Checked: true})
}

// createUser stores the user in memory. A real implementation would hash the
// password with bcrypt or argon2id and persist it.
func (s *Server) createUser(email, password string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.users[email]; exists {
		return fmt.Errorf("an account for %q already exists", email)
	}

	// Placeholder. Never store plaintext passwords in a real application.
	s.users[email] = "hashed:" + fmt.Sprint(len(password))

	return nil
}

// writeJSON serialises v as a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		// The status line is already committed; there is nothing left to do
		// but record it.
		slogError(err)
	}
}
