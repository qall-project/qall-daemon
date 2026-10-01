FROM docker-proxy.internal.scaleway.com/golang:1.26-alpine AS builder

RUN apk add --no-cache git

WORKDIR /workspace

COPY github.com/qall-project/qall-registry/server/go.mod github.com/qall-project/qall-registry/server/go.sum ./github.com/qall-project/qall-registry/server/
COPY qall-daemon-server/go.mod qall-daemon-server/go.sum ./qall-daemon-server/

WORKDIR /workspace/qall-daemon-server
RUN go mod download

WORKDIR /workspace
COPY github.com/qall-project/qall-registry/server/ ./github.com/qall-project/qall-registry/server/
COPY qall-daemon-server/ ./qall-daemon-server/

WORKDIR /workspace/qall-daemon-server
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/qall-daemon cmd/start_daemon.go

FROM docker-proxy.internal.scaleway.com/alpine:3.20

RUN apk add --no-cache ca-certificates

WORKDIR /app
COPY --from=builder /bin/qall-daemon .

EXPOSE 50053
EXPOSE 8080

ENTRYPOINT ["./qall-daemon"]