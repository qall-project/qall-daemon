package storage

import (
	"github.com/qall-project/qall-daemon/server/internal/datastore"
	"github.com/qall-project/qall-daemon/server/internal/registry"

	"github.com/qall-project/qall-registry/server/pkg/blockstores"
)

type StorageCollection struct {
	RegistryProvider   registry.RegistryProvider
	MetaStore          datastore.MetadataStore
	ArtifactBlockStore blockstores.BlockStore
	Close              func()
}

func NewStorage(isLocal bool, blockRegistryDir string, artifactRegistryDir string) (*StorageCollection, error) {
	if isLocal {
		return newLocalStorage(blockRegistryDir, artifactRegistryDir)
	}
	return newRemoteStorage(blockRegistryDir)
}
