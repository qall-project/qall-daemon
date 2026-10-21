package storage

import (
	"fmt"
	"log"
	"os"

	"github.com/qall-project/qall-daemon/server/internal/datastore"
	"github.com/qall-project/qall-daemon/server/internal/registry"

	bs "github.com/qall-project/qall-registry/server/pkg/blockstores/local"
	regCore "github.com/qall-project/qall-registry/server/pkg/core"
	ds "github.com/qall-project/qall-registry/server/pkg/datastores/local"
)

func newLocalStorage(blockRegistryDir string, artifactRegistryDir string) (*StorageCollection, error) {
	log.Printf("[LOCAL MODE] Blocks (RO): %s | Artifacts & DB (RW): %s", blockRegistryDir, artifactRegistryDir)

	for _, dir := range []string{blockRegistryDir, artifactRegistryDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create storage directory %q: %w", dir, err)
		}
	}

	// Init SQLite DB for tasks and artifacts metadata
	metaStore, err := datastore.NewSQLiteStore(artifactRegistryDir)
	if err != nil {
		return nil, fmt.Errorf("sqlite metadata store error: %w", err)
	}

	// Init artifact registry
	artifactBlockStore, err := bs.NewLocalBlockStore(artifactRegistryDir)
	if err != nil {
		metaStore.Close()
		return nil, fmt.Errorf("artifact block store error: %w", err)
	}

	// Init task registry
	blockStore, err := bs.NewLocalBlockStore(blockRegistryDir)
	if err != nil {
		metaStore.Close()
		return nil, fmt.Errorf("registry block store error: %w", err)
	}

	dataStore, err := ds.NewLocalDataStore(blockRegistryDir)
	if err != nil {
		metaStore.Close()
		return nil, fmt.Errorf("registry data store error: %w", err)
	}

	regCore := regCore.NewRegistryCore(blockStore, dataStore)
	provider := registry.NewLocalRegistry(regCore)

	cleanup := func() {
		log.Println("[Shutdown] Closing local databases...")
		metaStore.Close()
		dataStore.Close()
	}

	return &StorageCollection{
		RegistryProvider:   provider,
		MetaStore:          metaStore,
		ArtifactBlockStore: artifactBlockStore,
		Close:              cleanup,
	}, nil
}
