package core

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/google/uuid"

	"github.com/qall-project/qall-daemon/server/internal/object"
	"github.com/qall-project/qall-daemon/server/internal/runtime"
)

func (e *DaemonCore) RunTask(
	ctx context.Context,
	taskHash string,
	artifactHash string,
	providerCredentials map[string]string,
	workflowRunId string,
	dataloaderEntries []object.DataloaderEntry,
) (*object.TaskRun, error) {
	if taskHash == "" {
		return nil, fmt.Errorf("TaskHash can't be empty")
	}

	if workflowRunId == "" {
		return nil, fmt.Errorf("WorkflowRunId can't be empty: the daemon scopes each task's shared filesystem (/workdir) per workflow run, so a run identifier is mandatory")
	}

	// Cache the run's dataloader table (no-op for child submissions, which are
	// resolved against the entries registered by the parent request).
	e.registerRunDataloaders(workflowRunId, dataloaderEntries)

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

	providerName := "local"
	resourceName := "local"
	workerHash := ""
	assignment, exists := e.resourceAssignments[taskHash]

	if providerCredentials == nil {
		providerCredentials = make(map[string]string)
	}

	if exists {
		providerName = assignment.ResourceProvider
		workerHash = assignment.WorkerHash

		if len(assignment.ResourceNames) > 0 {
			resourceName = assignment.ResourceNames[0]
			providerCredentials["target_resource"] = resourceName
		}
	}

	envVariables := e.buildProviderEnvVars(providerName, providerCredentials)

	token := uuid.New().String()
	workerAddresses := []string{}
	var workerRun *object.WorkerRun

	if workerHash != "" {
		workerRun, err = e.startQuantumWorker(
			ctx,
			workerHash,
			taskRun.Id,
			envVariables,
			providerName,
		)

		if err != nil {
			_ = e.taskStore.DeleteTaskRun(ctx, taskRun.Id)

			return nil, fmt.Errorf(
				"failed to start quantum worker: %w",
				err,
			)
		}

		workerAddresses = append(workerAddresses, workerRun.Address)
	}

	taskRuntimeArgs := object.TaskRuntimeArgs{
		Payload:              taskPayload,
		TaskRunId:            taskRunId,
		WorkflowRunId:        workflowRunId,
		HostBlockRegistryDir: e.hostBlockRegistryDir,
		TaskHash:             taskHash,
		ArtifactHash:         artifactHash,
		Token:                token,
		WorkerAddresses:      workerAddresses,
		Name:                 fmt.Sprintf("qall-task-%s", taskRunId[:8]),
		EnvironmentVariables: envVariables,
		Dataloaders:          e.dataloadersForTask(workflowRunId, taskHash),
	}

	containerID, err := e.startTask(
		ctx,
		taskRuntimeArgs,
	)

	if err != nil {
		if workerRun != nil {
			e.stopWorkerRun(ctx, workerRun)
		}

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

		if workerRun != nil {
			e.stopWorkerRun(ctx, workerRun)
		}

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

		if err := os.Remove(tokenPath); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to remove token file %s for task run ID %s: %w", tokenPath, taskRunId, err)
		}

		dataloadersFilename := fmt.Sprintf("%s.dataloaders.json", taskRunId)
		dataloadersPath := filepath.Join(runtime.HostSecretsDir, dataloadersFilename)

		if err := os.Remove(dataloadersPath); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("dataloaders secrets file %s for task run ID %s not found", dataloadersPath, taskRunId)
		}
	}

	return &object.TaskRun{
		Id:     taskRunId,
		Status: status,
	}, nil
}
