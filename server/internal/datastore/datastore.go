package datastore

import (
	"context"

	"github.com/qall-project/qall-daemon/server/internal/object"
)

type MetadataStore interface {
	CreateWorkerRun(ctx context.Context, workerRun object.WorkerRun) (string, error)
	GetWorkerRun(ctx context.Context, workerRunID string) (object.WorkerRun, error)
	UpdateWorkerRun(ctx context.Context, workerRun object.WorkerRun) (object.WorkerRun, error)
	DeleteWorkerRun(ctx context.Context, workerRunID string) error
	ListWorkerRuns(ctx context.Context) ([]object.WorkerRun, error)
	ListWorkerRunsByTask(ctx context.Context, taskRunID string) ([]object.WorkerRun, error)

	CreateTaskRun(ctx context.Context, taskRun object.TaskRun) (string, error)
	GetTaskRun(ctx context.Context, taskRunID string) (object.TaskRun, error)
	UpdateTaskRun(ctx context.Context, taskRun object.TaskRun) (object.TaskRun, error)
	DeleteTaskRun(ctx context.Context, taskRunID string) error

	CreateArtifactMeta(ctx context.Context, meta object.ArtifactMeta) error
	GetArtifactMeta(ctx context.Context, hash string) ([]object.ArtifactMeta, error)
	ListArtifactsByTask(ctx context.Context, taskRunID string) ([]object.ArtifactMeta, error)

	Close() error
}
