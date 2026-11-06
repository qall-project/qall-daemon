package core

import (
	"context"
	"log"
	"sync"

	"github.com/qall-project/qall-daemon/server/internal/datastore"
	"github.com/qall-project/qall-daemon/server/internal/object"
	"github.com/qall-project/qall-daemon/server/internal/registry"
	"github.com/qall-project/qall-daemon/server/internal/runtime"
	"github.com/qall-project/qall-daemon/server/internal/watchdog"

	"github.com/qall-project/qall-registry/server/pkg/blockstores"
)

// taskDataloaderScope is the dataloader bookkeeping of one task run: the run it
// belongs to and the dataloader names whose volumes it references.
type taskDataloaderScope struct {
	workflowRunID string
	names         []string
}

// dataloaderVolumeRuntime is the optional part of the runtime contract used to
// manage dataloader volumes. It is type-asserted instead of added to
// runtime.Runtime so non-Docker runtimes (and test doubles) stay unaffected:
// without it, no volume is ever removed by the core.
type dataloaderVolumeRuntime interface {
	RemoveDataloaderVolumes(ctx context.Context, workflowRunID string, names []string) error
	ReapRunDataloaderVolumes(ctx context.Context, workflowRunID string) error
}

type DaemonCore struct {
	registry             registry.RegistryProvider
	hostBlockRegistryDir string
	taskStore            datastore.MetadataStore
	artifactStore        blockstores.BlockStore
	runtime              runtime.Runtime
	workers              []*object.WorkerEntry
	resourceAssignments  map[string]*object.ResourceAssignment
	watchdog             watchdog.WatchDog

	// dataloadersMu guards the dataloader bookkeeping below: it is read by
	// concurrent GetTaskStatus/task submissions and written on every run and
	// task transition.
	dataloadersMu sync.RWMutex
	// runDataloaders caches each workflow run's dataloader entries (grouped by
	// task_hash) as received on the root CreateTaskRunRequest, so child task
	// submissions can be resolved per container without re-sending the specs.
	runDataloaders map[string]map[string][]object.DataloaderSpec
	// runDataloaderVolumes holds the per-run, per-dataloader reference count of
	// started tasks: the volume of a dataloader is removed when the last
	// referencing task ends.
	runDataloaderVolumes map[string]map[string]int
	// taskDataloaderScopes maps a task run to the dataloader volumes it holds,
	// so GetTaskStatus can release them on DONE/ERROR.
	taskDataloaderScopes map[string]taskDataloaderScope
}

func NewDaemonCore(
	reg registry.RegistryProvider,
	hostBlockRegistryDir string,
	meta datastore.MetadataStore,
	bs blockstores.BlockStore,
	runtime runtime.Runtime,
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
		resourceAssignments:  make(map[string]*object.ResourceAssignment),
		runDataloaders:       make(map[string]map[string][]object.DataloaderSpec),
		runDataloaderVolumes: make(map[string]map[string]int),
		taskDataloaderScopes: make(map[string]taskDataloaderScope),
	}, nil
}

// removeDataloaderVolumes removes the given dataloader volumes of a run when the
// runtime supports it. It is best-effort: a volume that cannot be removed is
// swept by ReapRunDataloaderVolumes on shutdown.
func (e *DaemonCore) removeDataloaderVolumes(ctx context.Context, workflowRunID string, names []string) {
	if len(names) == 0 {
		return
	}

	volumeRuntime, ok := e.runtime.(dataloaderVolumeRuntime)
	if !ok {
		return
	}

	if err := volumeRuntime.RemoveDataloaderVolumes(ctx, workflowRunID, names); err != nil {
		log.Printf("[Dataloader] Failed to remove dataloader volumes %v of workflow run %s: %v", names, workflowRunID, err)
	}
}

// StopAllTasks gracefully stops every task/worker container managed by the
// daemon. It is invoked when the daemon is shutting down so that running tasks
// are killed before the daemon process exits. Dataloader volumes are reaped
// afterwards, run by run.
func (e *DaemonCore) StopAllTasks(ctx context.Context) error {
	stopErr := e.runtime.StopAll(ctx)

	if volumeRuntime, ok := e.runtime.(dataloaderVolumeRuntime); ok {
		for _, runID := range e.knownWorkflowRunIDs() {
			if err := volumeRuntime.ReapRunDataloaderVolumes(ctx, runID); err != nil {
				log.Printf("[Daemon-Shutdown] Failed to reap dataloader volumes of workflow run %s: %v", runID, err)

				if stopErr == nil {
					stopErr = err
				}
			}
		}
	}

	e.dataloadersMu.Lock()
	e.runDataloaderVolumes = make(map[string]map[string]int)
	e.taskDataloaderScopes = make(map[string]taskDataloaderScope)
	e.dataloadersMu.Unlock()

	return stopErr
}
