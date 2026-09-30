package storage

import (
	"fmt"
	"log"

	"qall-daemon-server/internal/datastore"
	"qall-daemon-server/internal/registry"

	bs "qall-registry-server/pkg/blockstores/local"
	regCore "qall-registry-server/pkg/core"
	ds "qall-registry-server/pkg/datastores/local"
)

func newLocalStorage() (*StorageCollection, error) {
	log.Printf("[LOCAL MODE] Blocks (RO): %s | Artifacts & DB (RW): %s", BlockRegistryDir, artifactRegistryDir)

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
	blockStore, err := bs.NewLocalBlockStore(BlockRegistryDir)
	if err != nil {
		metaStore.Close()
		return nil, fmt.Errorf("registry block store error: %w", err)
	}

	dataStore, err := ds.NewLocalDataStore(BlockRegistryDir)
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
