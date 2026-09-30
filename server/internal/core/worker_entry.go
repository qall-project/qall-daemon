package core

import (
	"context"

	"qall-daemon-server/internal/object"
)

func (e *DaemonCore) CreateWorkerEntry(ctx context.Context,
	provider string,
	hash string,
	inputFormat string) (*object.WorkerEntry, error) {
	worker := &object.WorkerEntry{
		Provider:    provider,
		Hash:        hash,
		InputFormat: inputFormat,
	}

	e.workers = append(e.workers, worker)

	return worker, nil
}
