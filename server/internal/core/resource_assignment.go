package core

import (
	"context"

	"github.com/qall-project/qall-daemon/server/internal/object"
)

func (e *DaemonCore) CreateResourceAssignment(ctx context.Context,
	resourceProvider string,
	taskHash string,
	resourceName string) (*object.ResourceAssigment, error) {
	assignment := &object.ResourceAssigment{
		ResourceProvider: resourceProvider,
		TaskHash:         taskHash,
		ResourceName:     resourceName,
	}

	e.resourceAssignments = append(e.resourceAssignments, assignment)

	return assignment, nil
}
