package core

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/qall-project/qall-daemon/server/internal/object"
)

const defaultImage = "docker-proxy.internal.scaleway.com/python:3.13-slim"

func (d *DaemonCore) startTask(ctx context.Context, args object.TaskRuntimeArgs) (string, error) {
	targetImage := args.Payload.Image

	if targetImage == "" {
		targetImage = defaultImage
	}

	customImageTag, err := d.runtime.CreateImage(ctx, "qall-task", targetImage, args.Payload.RuntimeQallVersion, args.Payload.Requirements)

	if err != nil {
		return "", err
	}

	containerID, err := d.runtime.StartTask(ctx, customImageTag, args)

	if err != nil {
		return "", fmt.Errorf("failed to start task: %w", err)
	}

	return containerID, nil
}

func (e *DaemonCore) startQuantumWorkers(
	ctx context.Context,
	inputFormats []string,
	taskRunId string,
	envVariables map[string]string,
	workerProvider string,
) ([]object.WorkerRun, error) {
	if inputFormats == nil || len(inputFormats) == 0 {
		return nil, nil
	}

	workerRuns := make([]object.WorkerRun, 0, len(inputFormats))

	for _, inputFormat := range inputFormats {
		log.Printf("[Core] Must start workers for provider %s with format %s", workerProvider, inputFormat)

		worker, err := e.findWorker(
			workerProvider,
			inputFormat,
		)

		if err != nil {
			e.stopWorkerRuns(ctx, workerRuns)
			return nil, err
		}

		workerRun := object.WorkerRun{
			TaskRunId: taskRunId,
		}

		workerRunId, err := e.taskStore.CreateWorkerRun(
			ctx,
			workerRun,
		)

		if err != nil {
			e.stopWorkerRuns(ctx, workerRuns)
			return nil, err
		}

		workerRun.Id = workerRunId

		containerId, address, err := e.startWorker(
			ctx,
			worker,
			workerRunId,
			taskRunId,
			envVariables,
		)

		if err != nil {
			e.stopWorkerRuns(ctx, workerRuns)

			return nil, fmt.Errorf(
				"failed to start worker for input format %q: %w",
				inputFormat,
				err,
			)
		}

		workerRun.ContainerId = containerId
		workerRun.Address = address

		_, err = e.taskStore.UpdateWorkerRun(
			ctx,
			workerRun,
		)

		workerRuns = append(
			workerRuns,
			workerRun,
		)
	}

	return workerRuns, nil
}

func (e *DaemonCore) startWorker(
	ctx context.Context,
	worker *object.WorkerEntry,
	workerRunId string,
	taskRunId string,
	envVariables map[string]string,
) (string, string, error) {
	workerPayload, err := getWorkerPayload(
		ctx,
		worker.WorkerHash,
		e.registry,
	)

	if err != nil {
		return "", "", fmt.Errorf(
			"failed to resolve worker payload %s: %w",
			worker.WorkerHash,
			err,
		)
	}

	targetImage := workerPayload.Image

	if targetImage == "" {
		targetImage = defaultImage
	}

	customImageTag, err := e.runtime.CreateImage(
		ctx,
		"qall-worker",
		targetImage,
		workerPayload.RuntimeQallVersion,
		workerPayload.Requirements,
	)

	if err != nil {
		return "", "", fmt.Errorf(
			"failed to create worker image: %w",
			err,
		)
	}

	if err != nil {
		return "", "", fmt.Errorf(
			"failed to get available TCP port: %w",
			err,
		)
	}

	args := object.WorkerRuntimeArgs{
		Payload:              workerPayload,
		TaskRunId:            taskRunId,
		Port:                 "50051",
		HostBlockRegistryDir: e.hostBlockRegistryDir,
		WorkerHash:           worker.Hash,
		WorkerRunId:          workerRunId,
		Name:                 fmt.Sprintf("qall-worker-%s", workerRunId[:8]),
		EnvironmentVariables: envVariables,
	}

	containerID, err := e.runtime.StartWorker(
		ctx,
		customImageTag,
		args,
	)

	if err != nil {
		return "", "", fmt.Errorf(
			"failed to start worker: %w",
			err,
		)
	}

	return containerID, fmt.Sprintf("%s:%s", args.Name, args.Port), nil
}

func (e *DaemonCore) findWorker(
	provider string,
	inputFormat string,
) (*object.WorkerEntry, error) {
	for _, worker := range e.workers {
		if worker.Provider == provider &&
			worker.InputFormat == inputFormat {
			return worker, nil
		}
	}

	return nil, fmt.Errorf(
		"no quantum worker registered for provider %q and input format %q",
		provider,
		inputFormat,
	)
}

func (e *DaemonCore) watchTaskRun(ctx context.Context, taskRun object.TaskRun) {
	watchdogCtx := context.WithoutCancel(ctx)

	_ = e.watchdog.Watch(
		taskRun.Id,
		func() bool {
			currentTaskRun, err := e.taskStore.GetTaskRun(watchdogCtx, taskRun.Id)

			if err != nil {
				log.Printf("[Watchdog Warning] Failed to get task run %s: %v", taskRun.Id, err)
				return false
			}

			status, err := e.runtime.GetStatus(watchdogCtx, currentTaskRun.ContainerId)

			if err != nil {
				log.Printf("[Watchdog Warning] Failed to inspect container %s: %v", currentTaskRun.ContainerId, err)
				return false
			}

			return status == object.StatusDone || status == object.StatusError
		},
		func() {
			log.Printf("[Watchdog] TaskRun %s completed. Stopping associated quantum workers...", taskRun.Id)
			if err := e.onTaskRunFinished(watchdogCtx, taskRun.Id); err != nil {
				log.Printf("[Watchdog Error] Failed to cleanup workers for task %s: %v", taskRun.Id, err)
			}
		},
	)
}

func (e *DaemonCore) onTaskRunFinished(
	ctx context.Context,
	taskRunID string,
) error {
	workers, err := e.taskStore.ListWorkerRunsByTask(
		ctx,
		taskRunID,
	)

	if err != nil {
		log.Printf(
			"[Warning] Failed to list workers for task %s: %v",
			taskRunID,
			err,
		)

		return err
	}

	e.stopWorkerRuns(ctx, workers)

	for _, worker := range workers {
		if err := e.taskStore.DeleteWorkerRun(
			ctx,
			worker.Id,
		); err != nil {
			log.Printf(
				"[Warning] Failed to delete worker run %s: %v",
				worker.Id,
				err,
			)

			return err
		}
	}

	return nil
}

func (e *DaemonCore) stopWorkerRuns(
	ctx context.Context,
	workers []object.WorkerRun,
) error {
	if workers == nil {
		return nil
	}

	for _, worker := range workers {
		err := e.runtime.Stop(
			ctx,
			worker.ContainerId,
		)

		if err != nil {
			return err
		}
	}

	return nil
}

func (e *DaemonCore) buildProviderEnvVars(providerName string, envs map[string]string) map[string]string {
	if len(envs) == 0 {
		return nil
	}

	envVars := make(map[string]string)
	cleanProvider := strings.ToUpper(strings.TrimSpace(providerName))

	envVars["QALL_PROVIDER"] = cleanProvider

	for rawKey, value := range envs {
		trimmedKey := strings.TrimSpace(rawKey)
		if trimmedKey == "" {
			continue
		}

		// Normalize key format (e.g., "secret-key" -> "SECRET_KEY")
		normalizedKey := strings.ToUpper(strings.ReplaceAll(trimmedKey, "-", "_"))

		envVars[fmt.Sprintf("QALL_PROVIDER_%s", normalizedKey)] = value

		if cleanProvider != "" {
			envVars[fmt.Sprintf("%s_%s=%s", cleanProvider, normalizedKey)] = value
			envVars[fmt.Sprintf("QALL_PROVIDER_%s_%s=%s", cleanProvider, normalizedKey)] = value
		}
	}

	return envVars
}
