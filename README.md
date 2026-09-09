# leakcheck-demo

Reference integration of [`hansestack-go/leakcheck`](https://github.com/hansestack/hansestack-go)
into a realistic Go backend.

It exists to demonstrate one rule: **a leak check that cannot complete must
never block a user from signing up or logging in.**

## Quick start

```sh
cp .env.skel .env       # then set LEAKCHECK_API_KEY
make up                 # docker compose up --build -d
open http://localhost:8080
```

Without Docker:

```sh
cp .env.skel .env
make run                # go run ./main.go serve
```

## Commands

### `serve` — demo signup backend

| Endpoint | Behaviour |
| --- | --- |
| `GET /` | HTML form for manual exploration |
| `GET /health` | Liveness. **Never** probes the Hansestack API |
| `POST /signup` | Creates a user, rejecting known-breached passwords |
| `POST /check` | Reports on a password without creating a user |

`POST /signup` outcomes:

| Scenario | Status | `leak_check` |
| --- | --- | --- |
| Password clean | `201` | `clean` |
| Password leaked | `400` | `leaked` |
| **Leak check failed** | **`201`** | `unavailable` |
| Malformed body | `400` | — |
| Email already exists | `409` | — |

The third row is the point of this demo. The user is created either way; the
only difference is one JSON field and a WARN log line. Nothing returns 5xx
because the leak check failed.

`GET /health` deliberately ignores the Hansestack API. An outage of a
supplementary security check must not mark your service unhealthy and get it
restarted or pulled from a load balancer.

### `check` — one-shot CLI lookup

```sh
echo -n 'hunter2' | go run ./main.go check
```

Reads from stdin by default so the password never lands in shell history or the
process list. Exit codes: `0` clean, `1` leaked, `2` configuration error.

## Demonstrating fail-open

```sh
make run ARGS=--simulate-outage
# or: go run ./main.go serve --simulate-outage

curl -X POST localhost:8080/signup \
  -H 'Content-Type: application/json' \
  -d '{"email":"a@b.c","password":"hunter2"}'
```

```json
{"status":"created","leak_check":"unavailable"}
```

HTTP 201, with a WARN in the logs. The signup survived a total outage of the
leak-check dependency.

## Configuration

All settings come from `.env` (via [cleanenv](https://github.com/ilyakaznacheev/cleanenv)),
with real environment variables taking precedence.

| Variable | Default | Description |
| --- | --- | --- |
| `LEAKCHECK_API_KEY` | — | **Required.** Get one at [portal.hansestack.de](https://portal.hansestack.de) |
| `LEAKCHECK_TIMEOUT` | `500ms` | Per-request timeout |
| `LEAKCHECK_FAIL_CLOSE` | `false` | Return errors instead of failing open |
| `LEAKCHECK_BREAKER_THRESHOLD` | `3` | Consecutive failures before the circuit breaker opens (`<=0` disables it) |
| `LEAKCHECK_BREAKER_COOLDOWN` | `30s` | How long the breaker stays open before letting a single probe request through |
| `LEAKCHECK_SERVE_PORT` | `8080` | HTTP port |
| `LEAKCHECK_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `LEAKCHECK_APP_ENVIRONMENT` | `development` | `production` switches slog to JSON |

> `LEAKCHECK_FAIL_CLOSE=true` is for batch jobs, CI and admin tooling only.
> Never enable it in an interactive authentication flow.

## The integration, in full

```go
res, err := s.leaks.CheckPassword(r.Context(), req.Password)
if err != nil {
	// FAIL OPEN. Log and continue as if the password were safe.
	s.logger.WarnContext(r.Context(), "leak check unavailable, continuing signup",
		"err", err, "outcome", res.Outcome)
}

if res.Leaked {
	// A confirmed leak is a validation failure: 4xx, not 5xx.
	writeJSON(w, http.StatusBadRequest, errorResponse{...})
	return
}
```

`CheckPassword` returns a `leakcheck.Result{ Leaked bool; Count int; Outcome
Outcome }`. `Leaked` and `Count` are only meaningful when `res.Outcome ==
leakcheck.OutcomeChecked`; for every other outcome (e.g. `skipped_timeout`,
`skipped_rate_limited`, `skipped_circuit_open`) they hold the neutral
fail-open values `false` / `0`. `Outcome` is what lets you tell "checked and
clean" apart from "check never ran" in logs and metrics, without changing the
fail-open policy.

With the default fail-open client `err` is **always nil** — the library already
caught the failure, logged it, and returned a neutral result. The `err` branch
is only reachable under `LEAKCHECK_FAIL_CLOSE=true` or `--simulate-outage`.
Handling it anyway keeps the code correct in both modes.

The client is also built with a circuit breaker
(`leakcheck.WithCircuitBreaker(threshold, cooldown)`, configured via
`LEAKCHECK_BREAKER_THRESHOLD` / `LEAKCHECK_BREAKER_COOLDOWN`): after
`threshold` consecutive unavailability failures it stops sending requests for
`cooldown` and fails open immediately instead of paying the full request
timeout on every call during an outage, then lets a single probe through once
the cooldown elapses.

## Privacy

The password never leaves the process. The library hashes it with SHA-1 locally
and transmits only the first 5 hex characters of the digest; the final
comparison happens in memory. See
[`llms.txt`](https://github.com/hansestack/hansestack-go/blob/main/llms.txt).

## Development

```sh
make help    # list targets
make test    # go test -v -race ./...
make lint    # golangci-lint
make down    # stop the compose stack
```

## License

[MIT](./LICENSE)
