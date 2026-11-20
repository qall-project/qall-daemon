package core

import (
	"context"

	"github.com/qall-project/qall-daemon/server/internal/object"
)

func (e *DaemonCore) CreateResourceAssignment(ctx context.Context,
	resourceProvider string,
	taskHash string,
	workerHash string,
	resourceName string) (*object.ResourceAssignment, error) {
	assignment := &object.ResourceAssignment{
		ResourceProvider: resourceProvider,
		TaskHash:         taskHash,
		WorkerHash:       workerHash,
		ResourceName:     resourceName,
	}

	e.resourceAssignments[taskHash] = assignment

	return assignment, nil
}
