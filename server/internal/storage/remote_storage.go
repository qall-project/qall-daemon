package storage

import (
	"fmt"
	"log"
)

func newRemoteStorage() (*StorageCollection, error) {
	log.Printf("[REMOTE MODE] Blocks (RO): %s | Remote Registry: %s", BlockRegistryDir)

	return nil, fmt.Errorf("remote mode is not fully implemented yet")
}
