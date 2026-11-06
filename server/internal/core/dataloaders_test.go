package core

import (
	"context"
	"testing"

	"github.com/qall-project/qall-daemon/server/internal/object"
)

func TestRegisterRunDataloaders_GroupsByTask(t *testing.T) {
	core, err := NewDaemonCore(nil, "/fake/dir", nil, nil, &mockContainerRuntime{})
	if err != nil {
		t.Fatalf("Error while creating daemon core: %v", err)
	}

	entries := []object.DataloaderEntry{
		{TaskHash: "task-a", DataloaderSpec: object.DataloaderSpec{Name: "mnist", Src: "s3://b/mnist", Dst: "/data/mnist"}},
		{TaskHash: "task-b", DataloaderSpec: object.DataloaderSpec{Name: "coco", Src: "s3://b/coco", Dst: "/data/coco"}},
		{TaskHash: "task-a", DataloaderSpec: object.DataloaderSpec{Name: "imagenet", Src: "s3://b/img", Dst: "/data/img"}},
		// Empty task hash entries are ignored.
		{TaskHash: "", DataloaderSpec: object.DataloaderSpec{Name: "orphan", Src: "s3://b/o", Dst: "/data/o"}},
	}

	core.registerRunDataloaders("run-1", entries)

	taskA := core.dataloadersForTask("run-1", "task-a")
	if len(taskA) != 2 {
		t.Fatalf("expected 2 specs for task-a, got %d", len(taskA))
	}
	if taskA[0].Name != "mnist" || taskA[1].Name != "imagenet" {
		t.Fatalf("unexpected task-a specs: %+v", taskA)
	}

	taskB := core.dataloadersForTask("run-1", "task-b")
	if len(taskB) != 1 || taskB[0].Name != "coco" {
		t.Fatalf("unexpected task-b specs: %+v", taskB)
	}

	orphan := core.dataloadersForTask("run-1", "")
	if len(orphan) != 0 {
		t.Fatalf("empty task hash entry should be ignored, got %+v", orphan)
	}
}

func TestRegisterRunDataloaders_NamespacedByRun(t *testing.T) {
	core, err := NewDaemonCore(nil, "/fake/dir", nil, nil, &mockContainerRuntime{})
	if err != nil {
		t.Fatalf("Error while creating daemon core: %v", err)
	}

	entries := []object.DataloaderEntry{
		{TaskHash: "task-a", DataloaderSpec: object.DataloaderSpec{Name: "first", Src: "s3://b/1", Dst: "/d1"}},
	}
	core.registerRunDataloaders("run-1", entries)

	// An empty registration (child submission) must not clobber the run table.
	core.registerRunDataloaders("run-1", nil)
	if got := core.dataloadersForTask("run-1", "task-a"); len(got) != 1 {
		t.Fatalf("run-1 table was clobbered: %+v", got)
	}

	// A different run id must not see run-1's entries.
	if got := core.dataloadersForTask("run-2", "task-a"); len(got) != 0 {
		t.Fatalf("run-2 should have no entries, got %+v", got)
	}
}

func TestAcquireReleaseRunDataloaderVolumes_RefCount(t *testing.T) {
	core, err := NewDaemonCore(nil, "/fake/dir", nil, nil, &mockContainerRuntime{})
	if err != nil {
		t.Fatalf("Error while creating daemon core: %v", err)
	}

	specs := []object.DataloaderSpec{
		{Name: "mnist", Src: "s3://b/mnist", Dst: "mnist"},
	}

	// Two tasks of the same run reference the same dataloader.
	first := core.acquireRunDataloaderVolumes("run-1", specs)
	if len(first) != 1 || first[0] != "mnist" {
		t.Fatalf("unexpected acquired names: %v", first)
	}

	second := core.acquireRunDataloaderVolumes("run-1", specs)
	if len(second) != 1 || second[0] != "mnist" {
		t.Fatalf("unexpected acquired names: %v", second)
	}

	// The first release must not free the volume: the second task still uses it.
	if released := core.releaseRunDataloaderVolumes(context.Background(), "run-1", first); len(released) != 0 {
		t.Fatalf("volume released too early: %v", released)
	}

	released := core.releaseRunDataloaderVolumes(context.Background(), "run-1", second)
	if len(released) != 1 || released[0] != "mnist" {
		t.Fatalf("expected mnist to be released, got %v", released)
	}

	// Releasing again is a no-op.
	if released := core.releaseRunDataloaderVolumes(context.Background(), "run-1", first); len(released) != 0 {
		t.Fatalf("unexpected release after the count reached zero: %v", released)
	}

	if _, known := core.runDataloaderVolumes["run-1"]; known {
		t.Fatalf("run-1 refcount table should be dropped once empty: %+v", core.runDataloaderVolumes)
	}
}

func TestAcquireReleaseRunDataloaderVolumes_RunIsolation(t *testing.T) {
	core, err := NewDaemonCore(nil, "/fake/dir", nil, nil, &mockContainerRuntime{})
	if err != nil {
		t.Fatalf("Error while creating daemon core: %v", err)
	}

	specs := []object.DataloaderSpec{
		{Name: "mnist", Src: "s3://b/mnist", Dst: "mnist"},
	}

	core.acquireRunDataloaderVolumes("run-1", specs)
	core.acquireRunDataloaderVolumes("run-2", specs)

	// Releasing run-1 must leave run-2's reference untouched.
	released := core.releaseRunDataloaderVolumes(context.Background(), "run-1", []string{"mnist"})
	if len(released) != 1 {
		t.Fatalf("expected run-1 to release mnist, got %v", released)
	}

	if count := core.runDataloaderVolumes["run-2"]["mnist"]; count != 1 {
		t.Fatalf("run-2 refcount = %d, want 1", count)
	}

	released = core.releaseRunDataloaderVolumes(context.Background(), "run-2", []string{"mnist"})
	if len(released) != 1 || released[0] != "mnist" {
		t.Fatalf("expected run-2 to release mnist, got %v", released)
	}
}

func TestAcquireRunDataloaderVolumes_DeduplicatesPerTask(t *testing.T) {
	core, err := NewDaemonCore(nil, "/fake/dir", nil, nil, &mockContainerRuntime{})
	if err != nil {
		t.Fatalf("Error while creating daemon core: %v", err)
	}

	specs := []object.DataloaderSpec{
		{Name: "mnist", Src: "s3://b/mnist", Dst: "mnist"},
		{Name: "mnist", Src: "s3://b/mnist", Dst: "mnist-alt"},
		{Name: "", Src: "s3://b/ignored", Dst: "ignored"},
	}

	names := core.acquireRunDataloaderVolumes("run-1", specs)
	if len(names) != 1 || names[0] != "mnist" {
		t.Fatalf("unexpected acquired names: %v", names)
	}

	if count := core.runDataloaderVolumes["run-1"]["mnist"]; count != 1 {
		t.Fatalf("refcount = %d, want 1 (a task references a dataloader once)", count)
	}

	released := core.releaseRunDataloaderVolumes(context.Background(), "run-1", names)
	if len(released) != 1 {
		t.Fatalf("expected a single release, got %v", released)
	}
}

func TestReleaseTaskDataloaderVolumes_ReleasesOnlyOnce(t *testing.T) {
	core, err := NewDaemonCore(nil, "/fake/dir", nil, nil, &mockContainerRuntime{})
	if err != nil {
		t.Fatalf("Error while creating daemon core: %v", err)
	}

	names := core.acquireRunDataloaderVolumes("run-1", []object.DataloaderSpec{
		{Name: "mnist", Src: "s3://b/mnist", Dst: "mnist"},
	})
	core.recordTaskDataloaderScope("task-1", "run-1", names)

	core.releaseTaskDataloaderVolumes(context.Background(), "task-1")
	if count := len(core.runDataloaderVolumes["run-1"]); count != 0 {
		t.Fatalf("refcount table should be empty after the release, got %+v", core.runDataloaderVolumes)
	}

	// A second terminal-status poll must not release anything again.
	core.releaseTaskDataloaderVolumes(context.Background(), "task-1")
	if _, ok := core.taskDataloaderScopes["task-1"]; ok {
		t.Fatal("task scope was not consumed")
	}
}

func TestKnownWorkflowRunIDs(t *testing.T) {
	core, err := NewDaemonCore(nil, "/fake/dir", nil, nil, &mockContainerRuntime{})
	if err != nil {
		t.Fatalf("Error while creating daemon core: %v", err)
	}

	core.registerRunDataloaders("run-1", []object.DataloaderEntry{
		{TaskHash: "task-a", DataloaderSpec: object.DataloaderSpec{Name: "mnist"}},
	})
	core.acquireRunDataloaderVolumes("run-2", []object.DataloaderSpec{{Name: "coco"}})

	ids := core.knownWorkflowRunIDs()
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		seen[id] = true
	}

	if !seen["run-1"] || !seen["run-2"] {
		t.Fatalf("knownWorkflowRunIDs = %v, want run-1 and run-2", ids)
	}
}

func TestStopAllTasks_ClearsDataloaderState(t *testing.T) {
	core, err := NewDaemonCore(nil, "/fake/dir", nil, nil, &mockContainerRuntime{})
	if err != nil {
		t.Fatalf("Error while creating daemon core: %v", err)
	}

	names := core.acquireRunDataloaderVolumes("run-1", []object.DataloaderSpec{{Name: "mnist"}})
	core.recordTaskDataloaderScope("task-1", "run-1", names)

	if err := core.StopAllTasks(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(core.runDataloaderVolumes) != 0 {
		t.Fatalf("runDataloaderVolumes not cleared: %+v", core.runDataloaderVolumes)
	}
	if len(core.taskDataloaderScopes) != 0 {
		t.Fatalf("taskDataloaderScopes not cleared: %+v", core.taskDataloaderScopes)
	}
}
