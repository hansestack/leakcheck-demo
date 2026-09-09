// Package config holds the runtime configuration for the demo, loaded from a
// .env file with environment variables taking precedence.
package config

import "time"

// Config is the top-level configuration tree.
type Config struct {
	App       AppConfig
	Log       LogConfig
	Serve     ServeConfig
	LeakCheck LeakCheckConfig
}

// AppConfig holds general application settings.
type AppConfig struct {
	Environment string `env:"LEAKCHECK_APP_ENVIRONMENT" env-default:"development"`
	ServiceName string `env:"LEAKCHECK_SERVICE_NAME"    env-default:"leakcheck-demo"`
}

// LogConfig controls slog verbosity.
type LogConfig struct {
	Level string `env:"LEAKCHECK_LOG_LEVEL" env-default:"info"`
}

// ServeConfig holds HTTP server settings for the demo backend.
type ServeConfig struct {
	Port string `env:"LEAKCHECK_SERVE_PORT" env-default:"8080"`
}

// LeakCheckConfig configures the Hansestack leak-check client.
//
// FailClose defaults to false. Do not enable it for interactive
// authentication flows: a leak check that cannot complete must never prevent a
// user from signing up or logging in. It exists here so the demo can show both
// behaviours side by side.
type LeakCheckConfig struct {
	APIKey    string        `env:"LEAKCHECK_API_KEY"    env-required:"true"`
	Timeout   time.Duration `env:"LEAKCHECK_TIMEOUT"    env-default:"500ms"`
	FailClose bool          `env:"LEAKCHECK_FAIL_CLOSE" env-default:"false"`

	// BreakerThreshold and BreakerCooldown configure the client's circuit
	// breaker (see leakcheck.WithCircuitBreaker). After BreakerThreshold
	// consecutive unavailability failures (timeouts, connection errors, 5xx,
	// 429) the client stops sending requests for BreakerCooldown and fails
	// open immediately, instead of paying the full request timeout on every
	// call during an outage. A non-positive threshold disables the breaker.
	BreakerThreshold int           `env:"LEAKCHECK_BREAKER_THRESHOLD" env-default:"3"`
	BreakerCooldown  time.Duration `env:"LEAKCHECK_BREAKER_COOLDOWN"  env-default:"30s"`
}
