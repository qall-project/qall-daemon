package core

import (
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
