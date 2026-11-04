package storage

import (
	"fmt"
	"log"
)

func newRemoteStorage(blockRegistryDir string) (*StorageCollection, error) {
	log.Printf("[REMOTE MODE] Blocks (RO): %s", blockRegistryDir)

	return nil, fmt.Errorf("remote mode is not fully implemented yet")
}
