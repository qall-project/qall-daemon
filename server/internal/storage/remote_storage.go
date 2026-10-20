package storage

import (
	"fmt"
	"log"
)

func newRemoteStorage() (*StorageCollection, error) {
	log.Printf("[REMOTE MODE] Blocks (RO): %s", BlockRegistryDir)

	return nil, fmt.Errorf("remote mode is not fully implemented yet")
}
