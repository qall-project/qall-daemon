package core

import (
	"context"

	"github.com/qall-project/qall-daemon/server/internal/datastore"
	"github.com/qall-project/qall-daemon/server/internal/object"
	"github.com/qall-project/qall-daemon/server/internal/registry"
	"github.com/qall-project/qall-daemon/server/internal/runtime"
	"github.com/qall-project/qall-daemon/server/internal/watchdog"

	"github.com/qall-project/qall-registry/server/pkg/blockstores"
)

type DaemonCore struct {
	registry             registry.RegistryProvider
	hostBlockRegistryDir string
	taskStore            datastore.MetadataStore
	artifactStore        blockstores.BlockStore
	runtime              runtime.Runtime
	workers              []*object.WorkerEntry
	resourceAssignments  map[string]*object.ResourceAssignment
	watchdog             watchdog.WatchDog
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
	}, nil
}

// StopAllTasks gracefully stops every task/worker container managed by the
// daemon. It is invoked when the daemon is shutting down so that running tasks
// are killed before the daemon process exits.
func (e *DaemonCore) StopAllTasks(ctx context.Context) error {
	return e.runtime.StopAll(ctx)
}
