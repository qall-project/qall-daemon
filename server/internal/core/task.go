package core

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/google/uuid"

	"github.com/qall-project/qall-daemon/server/object"
	"github.com/qall-project/qall-daemon/server/runtime"
)

func (e *DaemonCore) RunTask(
	ctx context.Context,
	taskHash string,
	artifactHash string,
) (*object.TaskRun, error) {
	if taskHash == "" {
		return nil, fmt.Errorf("TaskHash can't be empty")
	}

	taskPayload, err := getTaskPayload(
		ctx,
		taskHash,
		e.registry,
	)

	if err != nil {
		return nil, fmt.Errorf(
			"failed to resolve task payload: %w",
			err,
		)
	}

	taskRun := object.TaskRun{
		Status: object.StatusPending,
	}

	taskRunId, err := e.taskStore.CreateTaskRun(ctx, taskRun)
	taskRun.Id = taskRunId

	if err != nil {
		return nil, fmt.Errorf(
			"failed to persist task run: %w",
			err,
		)
	}

	workerRuns, err := e.startQuantumWorkers(
		ctx,
		taskPayload.QuantumRunInputFormats,
		taskRun.Id,
	)

	if err != nil {
		_ = e.taskStore.DeleteTaskRun(ctx, taskRun.Id)

		return nil, fmt.Errorf(
			"failed to start quantum workers: %w",
			err,
		)
	}

	token := uuid.New().String()

	workerAddresses := []string{}

	if workerRuns != nil && len(workerRuns) > 0 {
		for _, w := range workerRuns {
			workerAddresses = append(workerAddresses, w.Address)
		}
	}

	taskRuntimeArgs := object.TaskRuntimeArgs{
		Payload:              taskPayload,
		TaskRunId:            taskRunId,
		HostBlockRegistryDir: e.hostBlockRegistryDir,
		TaskHash:             taskHash,
		ArtifactHash:         artifactHash,
		Token:                token,
		WorkerAddresses:      workerAddresses,
		Name:                 fmt.Sprintf("qall-task-%s", taskRunId[:8]),
	}

	containerID, err := e.startTask(
		ctx,
		taskRuntimeArgs,
	)

	if err != nil {
		e.stopWorkerRuns(ctx, workerRuns)

		_ = e.taskStore.DeleteTaskRun(
			ctx,
			taskRun.Id,
		)

		return nil, fmt.Errorf(
			"runtime execution failed: %w",
			err,
		)
	}

	taskRun.ContainerId = containerID
	taskRun.Status = object.StatusRunning

	if _, err := e.taskStore.UpdateTaskRun(
		ctx,
		taskRun,
	); err != nil {
		e.runtime.Stop(ctx, containerID)
		e.stopWorkerRuns(ctx, workerRuns)

		return nil, fmt.Errorf(
			"failed to persist task container: %w",
			err,
		)
	}

	e.watchTaskRun(ctx, taskRun)

	return &taskRun, nil
}

func (e *DaemonCore) GetTaskStatus(ctx context.Context, taskRunId string) (*object.TaskRun, error) {
	taskRun, err := e.taskStore.GetTaskRun(ctx, taskRunId)

	if err != nil {
		return nil, fmt.Errorf("error querying task store: %w", err)
	}

	log.Printf("[Daemon] task run ID %s mapped to container ID %s", taskRunId, taskRun.ContainerId[:8])

	status, err := e.runtime.GetStatus(ctx, taskRun.ContainerId)

	if err != nil {
		return nil, fmt.Errorf("failed to check worker status: %w", err)
	}

	if status == object.StatusDone || status == object.StatusError {
		tokenFilename := fmt.Sprintf("%s.token", taskRunId)
		tokenPath := filepath.Join(runtime.HostSecretsDir, tokenFilename)

		if err := os.Remove(tokenPath); err != nil {
			return nil, fmt.Errorf("token file %s for task run ID %s not found", tokenPath, taskRunId)
		}
	}

	return &object.TaskRun{
		Id:     taskRunId,
		Status: status,
	}, nil
}
