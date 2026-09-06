# ==============================================================================
# Build Stage
# ==============================================================================
# BUILDPLATFORM = platform of the build host (e.g. linux/arm64 on Apple Silicon)
# TARGETPLATFORM / TARGETARCH / TARGETOS = platform we're building FOR
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder

# install git, ca-certificates and tzdata
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# copy source code
COPY . .

# build statically linked binary
# -ldflags="-w -s" strips debug information to minimize binary size
# TARGETOS/TARGETARCH are injected by docker buildx for cross-compilation
ARG TARGETOS=linux
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-w -s" -o /app/leakcheck-demo ./main.go

# ==============================================================================
# Final Stage
# ==============================================================================
# No --platform here: the final stage defaults to $TARGETPLATFORM already.
FROM alpine:latest

# ca-certificates is required: the demo makes TLS calls to api.hansestack.de
RUN apk add --no-cache ca-certificates tzdata wget

WORKDIR /app

# copy the binary
COPY --from=builder /app/leakcheck-demo .

# set environment variables
ENV LEAKCHECK_APP_ENVIRONMENT=production \
    LEAKCHECK_LOG_LEVEL=info \
    LEAKCHECK_SERVE_PORT=8080 \
    TZ=Europe/Berlin

# run as an unprivileged user
RUN adduser -D -H -u 10001 app
USER app

# expose the default HTTP port
EXPOSE 8080

# entrypoint directly invokes the binary, making the go process PID 1
ENTRYPOINT ["/app/leakcheck-demo"]
CMD ["serve"]
