package core

import (
	"context"

	"github.com/qall-project/qall-daemon/server/internal/object"
)

func (e *DaemonCore) CreateWorkerEntry(ctx context.Context,
	workerProvider string,
	workerHash string,
	inputFormat string) (*object.WorkerEntry, error) {
	worker := &object.WorkerEntry{
		WorkerProvider: workerProvider,
		WorkerHash:     workerHash,
		InputFormat:    inputFormat,
	}

	e.workers = append(e.workers, worker)

	return worker, nil
}
