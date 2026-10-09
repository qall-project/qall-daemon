package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/qall-project/qall-daemon/server/internal/core"
	"github.com/qall-project/qall-daemon/server/internal/runtime"
	"github.com/qall-project/qall-daemon/server/internal/server"
	"github.com/qall-project/qall-daemon/server/internal/storage"
)

type config struct {
	IsLocal              bool
	HostBlockRegistryDir string
	RemoteRegistryAddr   string
	GrpcPort             string
	HttpPort             string
}

func main() {
	cfg := parseFlags()
	ctx := context.Background()

	stores, err := storage.NewStorage(cfg.IsLocal)

	if err != nil {
		log.Fatalf("Fatal: Failed to initialize daemon storage: %v", err)
	}

	defer stores.Close()

	containerRuntime, err := runtime.NewDockerRuntime(ctx)

	if err != nil {
		log.Fatalf("Fatal: Failed to init container runtime: %v", err)
	}

	defer containerRuntime.Close()

	dcore, err := core.NewDaemonCore(
		stores.RegistryProvider,
		cfg.HostBlockRegistryDir,
		stores.MetaStore,
		stores.ArtifactBlockStore,
		containerRuntime,
	)

	if err != nil {
		log.Fatalf("Fatal: Failed to create daemon core: %v", err)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Printf("[Main] Received signal %v. Shutting down daemon...", sig)

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := dcore.Shutdown(shutdownCtx); err != nil {
			log.Printf("[Main Error] Graceful shutdown error: %v", err)
		}

		os.Exit(0)
	}()

	apiImpl := server.NewApiV1Server(dcore)

	log.Printf("[Daemon] Starting Qall daemon (Local Mode: %v)", cfg.IsLocal)

	if err := server.StartServer(ctx, apiImpl, cfg.GrpcPort, cfg.HttpPort); err != nil {
		log.Fatalf("Fatal: Network servers crashed: %v", err)
	}
}

func parseFlags() *config {
	cfg := &config{}

	flag.BoolVar(&cfg.IsLocal, "local", false, "Enable local mode")
	flag.StringVar(&cfg.HostBlockRegistryDir, "host-task-dir", ".registry-data", "Path to the Block Registry on the host machine")
	flag.StringVar(&cfg.RemoteRegistryAddr, "registry-addr", "127.0.0.1:50051", "Address of the remote registry")
	flag.StringVar(&cfg.GrpcPort, "grpc-port", "50053", "gRPC listening port")
	flag.StringVar(&cfg.HttpPort, "http-port", "8080", "HTTP (Gateway) listening port")
	flag.Parse()

	return cfg
}
