package storage

import (
	"github.com/qall-project/qall-daemon/server/datastore"
	"github.com/qall-project/qall-daemon/server/registry"

	"github.com/qall-project/qall-registry/server/pkg/blockstores"
)

type StorageCollection struct {
	RegistryProvider   registry.RegistryProvider
	MetaStore          datastore.MetadataStore
	ArtifactBlockStore blockstores.BlockStore
	Close              func()
}

const (
	BlockRegistryDir    = "/block-registry"
	artifactRegistryDir = "/artifact-registry"
)

func NewStorage(isLocal bool) (*StorageCollection, error) {
	if isLocal {
		return newLocalStorage()
	}

	return newRemoteStorage()
}
