FROM golang:1.26-alpine AS builder

RUN apk add --no-cache ca-certificates

WORKDIR /build

# Dependencies first for better layer caching
COPY server/go.mod server/go.sum ./
RUN go mod download

# Source code
COPY server/ ./

# Fully static Linux binary
RUN CGO_ENABLED=0 \
    GOOS=linux \
    go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /qall-daemon \
    ./cmd/start_daemon.go


FROM alpine:3.22

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /qall-daemon /app/qall-daemon

EXPOSE 50053
EXPOSE 8080

ENTRYPOINT ["/app/qall-daemon"]