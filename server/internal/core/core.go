package core

import (
	"context"
	"log"
	"sync"

	"qall-daemon-server/internal/datastore"
	"qall-daemon-server/internal/object"
	"qall-daemon-server/internal/registry"
	"qall-daemon-server/internal/runtime"
	"qall-daemon-server/internal/watchdog"

	"qall-registry-server/pkg/blockstores"
)

type DaemonCore struct {
	registry             registry.RegistryProvider
	hostBlockRegistryDir string
	taskStore            datastore.MetadataStore
	artifactStore        blockstores.BlockStore
	runtime              runtime.Runtime
	workers              []*object.WorkerEntry
	workerProvider       string
	watchdog             watchdog.WatchDog
}

func NewDaemonCore(
	reg registry.RegistryProvider,
	hostBlockRegistryDir string,
	meta datastore.MetadataStore,
	bs blockstores.BlockStore,
	runtime runtime.Runtime,
	workerProvider string,
) (*DaemonCore, error) {

	watchdog, err := watchdog.NewWatchdog(context.TODO())

	if err != nil {
		return nil, err
	}

	return &DaemonCore{
		registry:             reg,
		hostBlockRegistryDir: hostBlockRegistryDir,
		taskStore:            meta,
		artifactStore:        bs,
		runtime:              runtime,
		watchdog:             watchdog,
		workerProvider:       workerProvider,
	}, nil
}

func (e *DaemonCore) Shutdown(ctx context.Context) error {
	log.Println("[Daemon] Initiating graceful shutdown. Cleaning up active containers...")

	workers, err := e.taskStore.ListWorkerRuns(ctx)

	if err != nil {
		log.Printf("[Daemon Shutdown Warning] Could not query active workers: %v", err)
	}

	var wg sync.WaitGroup

	for _, w := range workers {
		if w.ContainerId == "" {
			continue
		}

		wg.Add(1)
		go func(containerID string) {
			defer wg.Done()
			log.Printf("[Daemon Shutdown] Stopping worker container %s...", containerID[:12])

			if err := e.runtime.Stop(ctx, containerID); err != nil {
				log.Printf("[Daemon Shutdown Warning] Failed to stop worker container %s: %v", containerID, err)
			}

		}(w.ContainerId)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("[Daemon] All worker containers stopped successfully.")
	case <-ctx.Done():
		log.Println("[Daemon Shutdown Warning] Shutdown timed out. Some containers may still be stopping.")
	}

	return nil
}
