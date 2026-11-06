package core

import (
	"context"

	"github.com/qall-project/qall-daemon/server/internal/object"
)

// registerRunDataloaders records the whole run's dataloader entries, keyed by
// task_hash, under workflowRunId. It is called on every CreateTaskRunRequest —
// the root request of a run carries the full Dag, while child submissions
// (from inside task containers, which have no keyring) carry an empty list and
// just resolve against the already-cached table. Entries with an empty
// task_hash are ignored.
func (e *DaemonCore) registerRunDataloaders(workflowRunId string, entries []object.DataloaderEntry) {
	if len(entries) == 0 {
		return
	}

	e.dataloadersMu.Lock()
	defer e.dataloadersMu.Unlock()

	byTask := e.runDataloaders[workflowRunId]
	if byTask == nil {
		byTask = make(map[string][]object.DataloaderSpec)
		e.runDataloaders[workflowRunId] = byTask
	}

	for _, entry := range entries {
		if entry.TaskHash == "" {
			continue
		}
		byTask[entry.TaskHash] = append(byTask[entry.TaskHash], entry.DataloaderSpec)
	}
}

// dataloadersForTask returns the dataloader specs belonging to the given task
// within a workflow run, as previously registered by registerRunDataloaders.
func (e *DaemonCore) dataloadersForTask(workflowRunId string, taskHash string) []object.DataloaderSpec {
	e.dataloadersMu.RLock()
	defer e.dataloadersMu.RUnlock()

	return e.runDataloaders[workflowRunId][taskHash]
}

// acquireRunDataloaderVolumes records that one more task references the
// dataloaders of the given specs and returns the (deduplicated) dataloader
// names. Each task referencing a dataloader gets its own reference: the volume
// is only removed once the last task of the run releases it.
func (e *DaemonCore) acquireRunDataloaderVolumes(runID string, specs []object.DataloaderSpec) []string {
	if len(specs) == 0 {
		return nil
	}

	e.dataloadersMu.Lock()
	defer e.dataloadersMu.Unlock()

	byName := e.runDataloaderVolumes[runID]
	if byName == nil {
		byName = make(map[string]int, len(specs))
		e.runDataloaderVolumes[runID] = byName
	}

	names := make([]string, 0, len(specs))
	seen := make(map[string]bool, len(specs))

	for _, spec := range specs {
		if spec.Name == "" || seen[spec.Name] {
			continue
		}
		seen[spec.Name] = true

		byName[spec.Name]++
		names = append(names, spec.Name)
	}

	return names
}

// releaseRunDataloaderVolumes decrements the reference count of every named
// dataloader of a run and returns the names whose count reached zero — the
// volumes the caller must remove from Docker. Unknown names are ignored so a
// double release (or a release after a reap) is a no-op.
//
// The context is accepted for symmetry with the runtime cleanup the callers
// perform right after a release.
func (e *DaemonCore) releaseRunDataloaderVolumes(_ context.Context, runID string, names []string) []string {
	if len(names) == 0 {
		return nil
	}

	e.dataloadersMu.Lock()
	defer e.dataloadersMu.Unlock()

	byName := e.runDataloaderVolumes[runID]
	if byName == nil {
		return nil
	}

	var released []string

	for _, name := range names {
		count, known := byName[name]
		if !known {
			continue
		}

		if count <= 1 {
			delete(byName, name)
			released = append(released, name)
			continue
		}

		byName[name] = count - 1
	}

	if len(byName) == 0 {
		delete(e.runDataloaderVolumes, runID)
	}

	return released
}

// recordTaskDataloaderScope remembers which dataloader volumes a task run
// references, so GetTaskStatus can release them when the task reaches a
// terminal state (the task run record itself carries neither the workflow run
// id nor the task hash).
func (e *DaemonCore) recordTaskDataloaderScope(taskRunID, workflowRunID string, names []string) {
	if taskRunID == "" || len(names) == 0 {
		return
	}

	e.dataloadersMu.Lock()
	defer e.dataloadersMu.Unlock()

	if e.taskDataloaderScopes == nil {
		e.taskDataloaderScopes = make(map[string]taskDataloaderScope)
	}

	e.taskDataloaderScopes[taskRunID] = taskDataloaderScope{
		workflowRunID: workflowRunID,
		names:         names,
	}
}

// takeTaskDataloaderScope returns and forgets the dataloader scope of a task
// run, so a terminal status is only released once.
func (e *DaemonCore) takeTaskDataloaderScope(taskRunID string) (taskDataloaderScope, bool) {
	e.dataloadersMu.Lock()
	defer e.dataloadersMu.Unlock()

	scope, ok := e.taskDataloaderScopes[taskRunID]
	if ok {
		delete(e.taskDataloaderScopes, taskRunID)
	}

	return scope, ok
}

// knownWorkflowRunIDs returns every workflow run id the daemon knows about,
// from both the dataloader and the volume-refcount tables.
func (e *DaemonCore) knownWorkflowRunIDs() []string {
	e.dataloadersMu.RLock()
	defer e.dataloadersMu.RUnlock()

	seen := make(map[string]bool, len(e.runDataloaders)+len(e.runDataloaderVolumes))
	ids := make([]string, 0, len(e.runDataloaders)+len(e.runDataloaderVolumes))

	for runID := range e.runDataloaders {
		if seen[runID] {
			continue
		}
		seen[runID] = true
		ids = append(ids, runID)
	}

	for runID := range e.runDataloaderVolumes {
		if seen[runID] {
			continue
		}
		seen[runID] = true
		ids = append(ids, runID)
	}

	return ids
}

// releaseTaskDataloaderVolumes releases the dataloader volumes held by a task
// run and removes the ones whose reference count reached zero. Failures to
// remove a Docker volume are only logged: the run reaper sweeps leftovers.
func (e *DaemonCore) releaseTaskDataloaderVolumes(ctx context.Context, taskRunID string) {
	scope, ok := e.takeTaskDataloaderScope(taskRunID)
	if !ok {
		return
	}

	released := e.releaseRunDataloaderVolumes(ctx, scope.workflowRunID, scope.names)
	if len(released) == 0 {
		return
	}

	e.removeDataloaderVolumes(ctx, scope.workflowRunID, released)
}
