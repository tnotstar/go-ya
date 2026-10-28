# ─── Build stage ─────────────────────────────────────────────
FROM golang:1.26-bookworm AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /ya-server ./cmd/ya-server

# ─── Runtime stage ────────────────────────────────────────────
FROM alpine:latest

RUN apk --no-cache add ca-certificates

COPY --from=builder /ya-server /usr/local/bin/ya-server

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/ya-server"]
CMD ["serve"]
