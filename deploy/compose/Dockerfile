# Multi-stage Dockerfile for ResonaMesh Autonomous P2P Node
# Stage 1: Build binary with pure-Go compiler
FROM golang:1.24-alpine AS builder

WORKDIR /src

# Install build tools
RUN apk add --no-cache git ca-certificates tzdata

# Cache Go modules
COPY go.mod go.sum ./
RUN go mod download

# Copy source code and pre-built frontend distribution
COPY . .

# Compile static standalone binary
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w -X main.AppVersion=0.1.0-alpha -X main.AppName=ResonaMesh" \
    -o /bin/resonamesh \
    ./cmd/resonamesh

# Stage 2: Minimal runtime image
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata curl

# Non-root unprivileged service user
RUN addgroup -S resonamesh && adduser -S resonamesh -G resonamesh -u 10001

WORKDIR /app
RUN mkdir -p /app/data && chown -R resonamesh:resonamesh /app

COPY --from=builder /bin/resonamesh /app/resonamesh

USER resonamesh:resonamesh

EXPOSE 8080 4001

HEALTHCHECK --interval=5s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -f http://127.0.0.1:8080/api/v1/status || exit 1

ENTRYPOINT ["/app/resonamesh"]
CMD ["--http", "0.0.0.0:8080", "--p2p", "4001", "--data", "/app/data"]
