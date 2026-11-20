package core

import (
	"context"

	"github.com/qall-project/qall-daemon/server/internal/object"
)

func (e *DaemonCore) CreateResourceAssignment(ctx context.Context,
	resourceProvider string,
	taskHash string,
	workerHash string,
	resourceNames []string) (*object.ResourceAssignment, error) {
	assignment := &object.ResourceAssignment{
		ResourceProvider: resourceProvider,
		TaskHash:         taskHash,
		WorkerHash:       workerHash,
		ResourceNames:    resourceNames,
	}

	e.resourceAssignments[taskHash] = assignment

	return assignment, nil
}
