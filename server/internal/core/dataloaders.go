package core

import (
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
	return e.runDataloaders[workflowRunId][taskHash]
}
