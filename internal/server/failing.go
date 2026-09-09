package server

import (
	"context"
	"errors"

	"github.com/hansestack/hansestack-go/leakcheck"
)

// errSimulatedOutage is returned by FailingChecker.
var errSimulatedOutage = errors.New("simulated leak-check outage")

// FailingChecker is a PasswordChecker whose every call fails.
//
// It backs the `serve --simulate-outage` flag, making the fail-open path
// observable without having to wait for a real incident. It returns the same
// shape the real client does under WithFailClose: a neutral Result alongside
// an error.
type FailingChecker struct{}

// NewFailingChecker returns a checker that always reports a failure.
func NewFailingChecker() *FailingChecker {
	return &FailingChecker{}
}

// CheckPassword always fails.
func (*FailingChecker) CheckPassword(_ context.Context, _ string) (leakcheck.Result, error) {
	return leakcheck.Result{Outcome: leakcheck.OutcomeSkippedError}, errSimulatedOutage
}
