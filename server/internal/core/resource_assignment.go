package core

import (
	"context"

	"github.com/qall-project/qall-daemon/server/internal/object"
)

func (e *DaemonCore) CreateResourceAssignment(ctx context.Context,
	resourceProvider string,
	taskHash string,
	resourceName string) (*object.ResourceAssignment, error) {
	assignment := &object.ResourceAssignment{
		ResourceProvider: resourceProvider,
		TaskHash:         taskHash,
		ResourceName:     resourceName,
	}

	e.resourceAssignments[taskHash] = assignment

	return assignment, nil
}
