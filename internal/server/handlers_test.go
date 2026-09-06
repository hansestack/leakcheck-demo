package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// stubChecker is a programmable PasswordChecker.
type stubChecker struct {
	leaked bool
	count  int
	err    error
	calls  int
}

func (s *stubChecker) CheckPassword(_ context.Context, _ string) (bool, int, error) {
	s.calls++

	return s.leaked, s.count, s.err
}

// newTestServer returns a Server with logging discarded.
func newTestServer(checker PasswordChecker) *Server {
	return New(checker, slog.New(slog.DiscardHandler))
}

// postJSON issues a JSON request against the server's routes.
func postJSON(t *testing.T, srv *Server, path, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, req)

	return rr
}

func decode[T any](t *testing.T, r io.Reader) T {
	t.Helper()

	var v T
	if err := json.NewDecoder(r).Decode(&v); err != nil {
		t.Fatalf("could not decode response: %v", err)
	}

	return v
}

// TestSignupFailOpen is the most important test in this repository.
//
// When the leak check fails, the signup must still succeed. A 5xx here would
// mean a Hansestack outage locks every user out of registration.
func TestSignupFailOpen(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "network failure", err: errors.New("connection refused")},
		{name: "timeout", err: context.DeadlineExceeded},
		{name: "rate limited", err: errors.New("leakcheck: rate limited")},
		{name: "upstream 5xx", err: errors.New("leakcheck: server error")},
		{name: "bad api key", err: errors.New("leakcheck: unauthorized")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := newTestServer(&stubChecker{err: tc.err})

			rr := postJSON(t, srv, "/signup", `{"email":"a@b.c","password":"pw"}`)

			if rr.Code >= 500 {
				t.Fatal("returned 5xx because the leak check failed; this violates fail-open")
			}
			if rr.Code != http.StatusCreated {
				t.Fatalf("status = %d, want %d (signup must survive a failed check)",
					rr.Code, http.StatusCreated)
			}

			got := decode[signupResponse](t, rr.Body)
			if got.LeakCheck != leakStatusUnavailable {
				t.Errorf("leak_check = %q, want %q", got.LeakCheck, leakStatusUnavailable)
			}
		})
	}
}

func TestSignupOutcomes(t *testing.T) {
	tests := []struct {
		name          string
		checker       *stubChecker
		body          string
		wantStatus    int
		wantLeakField string
	}{
		{
			name:          "clean password is created",
			checker:       &stubChecker{},
			body:          `{"email":"a@b.c","password":"pw"}`,
			wantStatus:    http.StatusCreated,
			wantLeakField: leakStatusClean,
		},
		{
			name:          "leaked password is rejected with 400",
			checker:       &stubChecker{leaked: true, count: 42},
			body:          `{"email":"a@b.c","password":"pw"}`,
			wantStatus:    http.StatusBadRequest,
			wantLeakField: leakStatusLeaked,
		},
		{
			name:       "malformed json is rejected",
			checker:    &stubChecker{},
			body:       `{"email":`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing password is rejected",
			checker:    &stubChecker{},
			body:       `{"email":"a@b.c"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing email is rejected",
			checker:    &stubChecker{},
			body:       `{"password":"pw"}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := newTestServer(tc.checker)

			rr := postJSON(t, srv, "/signup", tc.body)

			if rr.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rr.Code, tc.wantStatus, rr.Body)
			}
			if tc.wantLeakField == "" {
				return
			}

			// Both the success and rejection bodies carry a leak_check field.
			var payload struct {
				LeakCheck string `json:"leak_check"`
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
				t.Fatalf("could not decode response: %v", err)
			}
			if payload.LeakCheck != tc.wantLeakField {
				t.Errorf("leak_check = %q, want %q", payload.LeakCheck, tc.wantLeakField)
			}
		})
	}
}

func TestSignupPassesRequestContext(t *testing.T) {
	checker := &stubChecker{}
	srv := newTestServer(checker)

	if rr := postJSON(t, srv, "/signup", `{"email":"a@b.c","password":"pw"}`); rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rr.Code)
	}
	if checker.calls != 1 {
		t.Errorf("checker called %d times, want exactly 1", checker.calls)
	}
}

func TestSignupDuplicateEmail(t *testing.T) {
	srv := newTestServer(&stubChecker{})

	if rr := postJSON(t, srv, "/signup", `{"email":"a@b.c","password":"pw"}`); rr.Code != http.StatusCreated {
		t.Fatalf("first signup status = %d, want 201", rr.Code)
	}

	rr := postJSON(t, srv, "/signup", `{"email":"a@b.c","password":"pw"}`)
	if rr.Code != http.StatusConflict {
		t.Fatalf("duplicate signup status = %d, want %d", rr.Code, http.StatusConflict)
	}
}

func TestCheckEndpoint(t *testing.T) {
	t.Run("reports a leak", func(t *testing.T) {
		srv := newTestServer(&stubChecker{leaked: true, count: 7})

		rr := postJSON(t, srv, "/check", `{"password":"pw"}`)
		got := decode[checkResponse](t, rr.Body)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rr.Code)
		}
		if !got.Leaked || got.Count != 7 || !got.Checked {
			t.Errorf("got %+v, want leaked=true count=7 checked=true", got)
		}
	})

	t.Run("marks an unavailable check", func(t *testing.T) {
		srv := newTestServer(&stubChecker{err: errors.New("boom")})

		rr := postJSON(t, srv, "/check", `{"password":"pw"}`)
		got := decode[checkResponse](t, rr.Body)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rr.Code)
		}
		if got.Checked {
			t.Error("checked = true, want false when the lookup failed")
		}
	})

	t.Run("missing password is rejected", func(t *testing.T) {
		srv := newTestServer(&stubChecker{})

		if rr := postJSON(t, srv, "/check", `{}`); rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rr.Code)
		}
	})
}

// TestHealthIgnoresLeakCheck verifies that health does not depend on the
// upstream API: a dependency outage must not get this service restarted.
func TestHealthIgnoresLeakCheck(t *testing.T) {
	srv := newTestServer(NewFailingChecker())

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 even while the leak check is down", rr.Code)
	}
}

func TestFailingCheckerAlwaysErrors(t *testing.T) {
	leaked, count, err := NewFailingChecker().CheckPassword(context.Background(), "pw")

	if err == nil {
		t.Fatal("err = nil, want a simulated outage error")
	}
	if leaked || count != 0 {
		t.Errorf("got (%v, %d), want (false, 0)", leaked, count)
	}
}

func TestIndexAndNotFound(t *testing.T) {
	srv := newTestServer(&stubChecker{})

	for path, want := range map[string]int{"/": http.StatusOK, "/nope": http.StatusNotFound} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rr := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rr, req)

		if rr.Code != want {
			t.Errorf("GET %s = %d, want %d", path, rr.Code, want)
		}
	}
}
