package cli

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hansestack/leakcheck-demo/internal/server"
	"github.com/spf13/cobra"
)

var simulateOutage bool

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "run the demo signup backend",
	Long: `serve starts a small HTTP backend that demonstrates leak-check
integration in a signup flow.

Endpoints:
  GET  /         HTML form for manual exploration
  GET  /health   liveness probe (never probes the Hansestack API)
  POST /signup   create a user, rejecting known-breached passwords
  POST /check    report on a password without creating a user

Use --simulate-outage to point the client at an unroutable address and observe
that signups still succeed while the leak check is unavailable.`,
	RunE: runServe,
}

func init() {
	serveCmd.Flags().BoolVar(&simulateOutage, "simulate-outage", false,
		"make every leak check fail, to demonstrate fail-open behaviour")
}

func runServe(_ *cobra.Command, _ []string) error {
	logger := slog.Default()

	var checker server.PasswordChecker = newClient()
	if simulateOutage {
		logger.Warn("simulating a leak-check outage: every check will fail")
		checker = server.NewFailingChecker()
	}

	srv := &http.Server{ //nolint:gosec // G112: ReadHeaderTimeout is set explicitly
		Addr:              ":" + appCfg.Serve.Port,
		Handler:           server.New(checker, logger).Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Wait for a termination signal and initiate a graceful 5-second shutdown.
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)

		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan
		logger.Info("received shutdown signal")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("graceful shutdown failed", "err", err)
		}
	}()

	// .String() keeps the JSON handler from rendering the duration as raw
	// nanoseconds.
	logger.Info("listening",
		"port", appCfg.Serve.Port,
		"timeout", appCfg.LeakCheck.Timeout.String(),
		"fail_close", appCfg.LeakCheck.FailClose,
	)

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server error", "err", err)

		return err
	}

	<-shutdownDone
	logger.Info("shutdown complete")

	return nil
}
