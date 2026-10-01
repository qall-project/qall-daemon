package runtime

import (
	"context"

	"github.com/qall-project/qall-daemon/server/internal/object"
)

type Runtime interface {
	CreateImage(ctx context.Context, targetImageName string, baseImageName string, requirements []string) (string, error)
	StartTask(ctx context.Context, image string, args object.TaskRuntimeArgs) (string, error)
	GetStatus(ctx context.Context, containerID string) (object.RuntimeStatus, error)
	StartWorker(ctx context.Context, image string, args object.WorkerRuntimeArgs) (string, error)
	Stop(ctx context.Context, id string) error
	Close() error
}
