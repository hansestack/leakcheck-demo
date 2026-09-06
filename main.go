// Command leakcheck-demo is a reference integration of the Hansestack
// leak-check client (github.com/hansestack/hansestack-go/leakcheck) into a
// realistic Go backend.
//
// It exists to demonstrate the fail-open contract: a leak check that cannot
// complete must never block a user from signing up or logging in.
package main

import (
	"log/slog"
	"os"

	"github.com/hansestack/leakcheck-demo/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		slog.Error("fatal error executing command", "err", err)
		os.Exit(1)
	}
}
