package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
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
	BlockRegistryDir     string
	ArtifactRegistryDir  string
	RemoteRegistryAddr   string
	DaemonAddr           string
	GrpcPort             string
	HttpPort             string
}

func main() {
	cfg := parseFlags()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	stores, err := storage.NewStorage(
		cfg.IsLocal,
		cfg.BlockRegistryDir,
		cfg.ArtifactRegistryDir,
	)

	if err != nil {
		log.Fatalf("Fatal: Failed to initialize daemon storage: %v", err)
	}

	defer stores.Close()

	containerRuntime, err := runtime.NewDockerRuntime(ctx, runtime.Options{
		DaemonAddr:     cfg.DaemonAddr,
		DaemonGrpcPort: cfg.GrpcPort,
	})

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

	apiImpl := server.NewApiV1Server(dcore)

	log.Printf("[Daemon] Starting Qall daemon (Local Mode: %v)", cfg.IsLocal)

	errCh := make(chan error, 1)

	go func() {
		errCh <- server.StartServer(ctx, apiImpl, cfg.GrpcPort, cfg.HttpPort)
	}()

	select {
	case err := <-errCh:
		log.Fatalf("Fatal: Network servers crashed: %v", err)
	case <-ctx.Done():
		log.Printf("[Daemon] Received shutdown signal, gracefully stopping task containers...")

		stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := dcore.StopAllTasks(stopCtx); err != nil {
			log.Printf("[Daemon] Failed to gracefully stop all task containers: %v", err)
		}
		log.Printf("[Daemon] Shutdown complete.")
	}
}

func parseFlags() *config {
	cfg := &config{}

	flag.BoolVar(&cfg.IsLocal, "local", false, "Enable local mode")
	flag.StringVar(&cfg.HostBlockRegistryDir, "host-task-dir", defaultCacheDir("block-registry"), "Host path to the Block Registry, bind-mounted into task/worker containers as /registry")
	flag.StringVar(&cfg.BlockRegistryDir, "block-registry-dir", defaultCacheDir("block-registry"), "Directory the daemon reads its local block registry from")
	flag.StringVar(&cfg.ArtifactRegistryDir, "artifact-registry-dir", defaultCacheDir("artifact-registry"), "Directory the daemon writes artifacts and its metadata DB to")
	flag.StringVar(&cfg.RemoteRegistryAddr, "registry-addr", "127.0.0.1:50051", "Address of the remote registry")
	flag.StringVar(&cfg.DaemonAddr, "daemon-addr", "", "Address (host or IP) task/worker containers use to reach this daemon; defaults to the bridge gateway when run natively on the host, or the container hostname when containerized")
	flag.StringVar(&cfg.GrpcPort, "grpc-port", "50053", "gRPC listening port")
	flag.StringVar(&cfg.HttpPort, "http-port", "8080", "HTTP (Gateway) listening port")
	flag.Parse()

	return cfg
}

// defaultCacheDir returns $HOME/.cache/qall/<name> when HOME is available
func defaultCacheDir(name string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".cache", "qall", name)
	}
	return filepath.Join(".cache", "qall", name)
}
