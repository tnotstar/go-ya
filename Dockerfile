# ─── Build stage ─────────────────────────────────────────────
FROM golang:1.26-bookworm AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o /gateway ./cmd/gateway

# ─── Runtime stage ────────────────────────────────────────────
# Use debian-slim for CGO (sqlite3 requires it).
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
  && rm -rf /var/lib/apt/lists/*

COPY --from=builder /gateway /usr/local/bin/gateway

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/gateway"]
CMD ["serve"]
