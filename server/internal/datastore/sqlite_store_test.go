package datastore

import (
	"context"
	"testing"
	"time"

	"qall-daemon-server/internal/object"
)

func setupTestDB(t *testing.T) *SQLiteStore {
	t.Helper()
	tempDir := t.TempDir()
	store, err := NewSQLiteStore(tempDir)
	if err != nil {
		t.Fatalf("Failed to initialize test SQLite store: %v", err)
	}
	return store
}

func TestTaskContainerUpdate(t *testing.T) {
	store := setupTestDB(t)
	defer store.Close()

	ctx := context.Background()
	taskRunID := "task-123"
	containerID := "container-abc"
	updatedContainerID := "container-xyz"

	err := store.SaveTaskContainer(ctx, taskRunID, containerID)
	if err != nil {
		t.Fatalf("Failed to save task container: %v", err)
	}

	retrievedID, exists, err := store.GetTaskContainer(ctx, taskRunID)
	if err != nil || !exists {
		t.Fatalf("Failed to get task container, exists: %v, err: %v", exists, err)
	}

	if retrievedID != containerID {
		t.Errorf("Expected container ID %s, got %s", containerID, retrievedID)
	}

	err = store.SaveTaskContainer(ctx, taskRunID, updatedContainerID)

	if err != nil {
		t.Fatalf("Failed to update task container: %v", err)
	}
}

func TestTaskContainerDelete(t *testing.T) {
	store := setupTestDB(t)
	defer store.Close()

	ctx := context.Background()
	taskRunID := "task-test"
	containerID := "container-abc"

	err := store.SaveTaskContainer(ctx, taskRunID, containerID)
	if err != nil {
		t.Fatalf("Failed to save task container: %v", err)
	}

	retrievedID, _, _ := store.GetTaskContainer(ctx, taskRunID)
	if retrievedID != containerID {
		t.Errorf("Expected updated container ID %s, got %s", containerID, retrievedID)
	}

	err = store.DeleteTaskContainer(ctx, taskRunID)
	if err != nil {
		t.Fatalf("Failed to delete task container: %v", err)
	}

	_, exists, err := store.GetTaskContainer(ctx, taskRunID)
	if err != nil || exists {
		t.Errorf("Expected task to be deleted, exists: %v, err: %v", exists, err)
	}
}

func TestArtifactMeta_OneTask(t *testing.T) {
	store := setupTestDB(t)
	defer store.Close()

	ctx := context.Background()
	taskRunID := "task-test"
	artifactHash := "hash-test"
	now := time.Now().UTC()

	meta := object.ArtifactMeta{
		Hash:      artifactHash,
		TaskRunID: taskRunID,
		CreatedAt: now,
	}

	err := store.SaveTaskContainer(ctx, taskRunID, "container-def")
	if err != nil {
		t.Fatalf("Failed to create parent task: %v", err)
	}

	err = store.SaveArtifactMeta(ctx, meta)
	if err != nil {
		t.Fatalf("Failed to save artifact meta: %v", err)
	}

	retrievedMetas, err := store.GetArtifactMeta(ctx, artifactHash)
	if err != nil {
		t.Fatalf("Failed to get artifact meta: %v", err)
	}

	if len(retrievedMetas) != 1 {
		t.Fatalf("Expected exactly 1 metadata entry, got %d", len(retrievedMetas))
	}
	if retrievedMetas[0].Hash != meta.Hash || retrievedMetas[0].TaskRunID != meta.TaskRunID {
		t.Errorf("Metadata mismatch. Expected %+v, got %+v", meta, retrievedMetas[0])
	}

	list, err := store.ListArtifactsByTask(ctx, taskRunID)
	if err != nil {
		t.Fatalf("Failed to list artifacts: %v", err)
	}
	if len(list) != 1 || list[0].Hash != artifactHash {
		t.Errorf("Expected 1 artifact in list with hash %s, got %v", artifactHash, list)
	}
}

func TestArtifactMeta_MultipleTask(t *testing.T) {
	store := setupTestDB(t)
	defer store.Close()

	ctx := context.Background()
	taskRunID_1 := "task-abc"
	taskRunID_2 := "task-xyz"
	artifactHash := "hash-test"
	now := time.Now().UTC()

	meta_1 := object.ArtifactMeta{
		Hash:      artifactHash,
		TaskRunID: taskRunID_1,
		CreatedAt: now,
	}

	meta_2 := object.ArtifactMeta{
		Hash:      artifactHash,
		TaskRunID: taskRunID_2,
		CreatedAt: now,
	}

	err := store.SaveTaskContainer(ctx, taskRunID_1, "container-def-1")
	if err != nil {
		t.Fatalf("Failed to create parent task: %v", err)
	}
	err = store.SaveTaskContainer(ctx, taskRunID_2, "container-def-2")
	if err != nil {
		t.Fatalf("Failed to create parent task: %v", err)
	}

	// Save artifact meta of the 1st task
	err = store.SaveArtifactMeta(ctx, meta_1)
	if err != nil {
		t.Fatalf("Failed to save artifact meta_1: %v", err)
	}

	// Save artifact meta of the 2nd task
	err = store.SaveArtifactMeta(ctx, meta_2)
	if err != nil {
		t.Fatalf("Failed to save artifact meta_2: %v", err)
	}

	retrievedMetas, err := store.GetArtifactMeta(ctx, artifactHash)
	if err != nil {
		t.Fatalf("Failed to get artifact metas: %v", err)
	}

	if len(retrievedMetas) != 2 {
		t.Fatalf("Expected 2 metadata entries for hash %s, got %d", artifactHash, len(retrievedMetas))
	}

	foundTask1 := false
	foundTask2 := false
	for _, rm := range retrievedMetas {
		if rm.TaskRunID == taskRunID_1 {
			foundTask1 = true
		}
		if rm.TaskRunID == taskRunID_2 {
			foundTask2 = true
		}
	}

	if !foundTask1 || !foundTask2 {
		t.Errorf("GetArtifactMeta did not return both expected TaskRunIDs. Got: %+v", retrievedMetas)
	}
}

func TestArtifactMeta_NoTask(t *testing.T) {
	store := setupTestDB(t)
	defer store.Close()

	ctx := context.Background()
	taskRunID := "task-test"
	artifactHash := "hash-test"
	now := time.Now().UTC()

	meta := object.ArtifactMeta{
		Hash:      artifactHash,
		TaskRunID: taskRunID,
		CreatedAt: now,
	}

	err := store.SaveArtifactMeta(ctx, meta)
	if err == nil {
		t.Fatal("Expected FOREIGN KEY constraint failure when inserting artifact without existing task")
	}
}

func TestArtifactMeta_DeleteTask(t *testing.T) {
	store := setupTestDB(t)
	defer store.Close()

	ctx := context.Background()
	taskRunID := "task-test"
	artifactHash := "hash-test"
	now := time.Now().UTC()

	meta := object.ArtifactMeta{
		Hash:      artifactHash,
		TaskRunID: taskRunID,
		CreatedAt: now,
	}

	err := store.SaveTaskContainer(ctx, taskRunID, "container-def")
	if err != nil {
		t.Fatalf("Failed to create parent task: %v", err)
	}

	err = store.SaveArtifactMeta(ctx, meta)
	if err != nil {
		t.Fatalf("Failed to save artifact meta: %v", err)
	}

	err = store.DeleteTaskContainer(ctx, taskRunID)
	if err != nil {
		t.Fatalf("Failed to delete parent task: %v", err)
	}

	_, err = store.GetArtifactMeta(ctx, artifactHash)
	if err == nil {
		t.Fatal("Expected artifact to be deleted via CASCADE, but it still exists")
	}
}

func TestGetNonExistentData(t *testing.T) {
	store := setupTestDB(t)
	defer store.Close()
	ctx := context.Background()

	_, exists, err := store.GetTaskContainer(ctx, "ghost-task")
	if err != nil || exists {
		t.Errorf("Expected no error and exists=false, got exists=%v, err=%v", exists, err)
	}

	_, err = store.GetArtifactMeta(ctx, "ghost-hash")
	if err == nil {
		t.Error("Expected error when fetching non-existent artifact meta, got nil")
	}
}
