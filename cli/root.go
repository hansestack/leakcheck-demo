// Package cli wires up the cobra command tree and shared configuration.
package cli

import (
	"log"
	"log/slog"
	"os"

	"github.com/hansestack/leakcheck-demo/internal/config"
	"github.com/ilyakaznacheev/cleanenv"
	"github.com/spf13/cobra"
)

// Exit codes used by the check command, chosen so the binary is scriptable.
const (
	exitClean  = 0 // password not found in any known breach
	exitLeaked = 1 // password found in the breach corpus
	exitError  = 2 // configuration or input error
)

var appCfg config.Config

var rootCmd = &cobra.Command{
	Use:   "leakcheck-demo",
	Short: "reference integration of the Hansestack leak-check client",
	Long: `leakcheck-demo demonstrates how to integrate the Hansestack leak-check
client into a Go backend.

The guiding rule is fail-open: a leak check that cannot complete must never
prevent a user from signing up, logging in or changing a password.`,
	SilenceUsage: true,
	PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
		setupLogger(appCfg.Log.Level, appCfg.App.Environment)

		return nil
	},
}

// Execute is called by main.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.AddCommand(checkCmd)
	rootCmd.AddCommand(serveCmd)
}

// initConfig loads config from a .env file (local dev) with env vars taking
// precedence. Falls back to pure env vars when no .env file is found.
// Fatals immediately if any env-required variable is missing.
func initConfig() {
	if err := cleanenv.ReadConfig(".env", &appCfg); err != nil {
		if err2 := cleanenv.ReadEnv(&appCfg); err2 != nil {
			log.Fatalf("configuration error: %v\n\n"+
				"LEAKCHECK_API_KEY is required. Copy .env.skel to .env and fill it in,\n"+
				"or export LEAKCHECK_API_KEY in your shell.", err2)
		}
	}
}

// setupLogger initializes the global slog logger based on environment and level.
func setupLogger(level, env string) {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}

	var handler slog.Handler
	if env == "production" {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})
	}

	slog.SetDefault(slog.New(handler))
}
