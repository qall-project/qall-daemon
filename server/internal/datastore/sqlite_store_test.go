package datastore

import (
	"context"
	"testing"
	"time"

	"github.com/qall-project/qall-daemon/server/internal/object"
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

func createTaskRun(t *testing.T, store *SQLiteStore, ctx context.Context, containerID string) string {
	t.Helper()
	id, err := store.CreateTaskRun(
		ctx,
		object.TaskRun{ContainerId: containerID, Status: object.StatusPending},
	)
	if err != nil {
		t.Fatalf("Failed to create task run: %v", err)
	}
	return id
}

func TestTaskRunUpdate(t *testing.T) {
	store := setupTestDB(t)
	defer store.Close()

	ctx := context.Background()
	taskRunID := createTaskRun(t, store, ctx, "container-abc")
	updatedContainerID := "container-xyz"

	retrieved, err := store.GetTaskRun(ctx, taskRunID)
	if err != nil {
		t.Fatalf("Failed to get task run: %v", err)
	}

	if retrieved.ContainerId != "container-abc" {
		t.Errorf("Expected container ID %s, got %s", "container-abc", retrieved.ContainerId)
	}

	retrieved.ContainerId = updatedContainerID
	if _, err := store.UpdateTaskRun(ctx, retrieved); err != nil {
		t.Fatalf("Failed to update task run: %v", err)
	}

	updated, err := store.GetTaskRun(ctx, taskRunID)
	if err != nil {
		t.Fatalf("Failed to get task run after update: %v", err)
	}

	if updated.ContainerId != updatedContainerID {
		t.Errorf("Expected updated container ID %s, got %s", updatedContainerID, updated.ContainerId)
	}
}

func TestTaskRunDelete(t *testing.T) {
	store := setupTestDB(t)
	defer store.Close()

	ctx := context.Background()
	taskRunID := createTaskRun(t, store, ctx, "container-abc")

	if err := store.DeleteTaskRun(ctx, taskRunID); err != nil {
		t.Fatalf("Failed to delete task run: %v", err)
	}

	if _, err := store.GetTaskRun(ctx, taskRunID); err == nil {
		t.Errorf("Expected task run to be deleted, got no error")
	}
}

func TestArtifactMeta_OneTask(t *testing.T) {
	store := setupTestDB(t)
	defer store.Close()

	ctx := context.Background()
	taskRunID := createTaskRun(t, store, ctx, "container-def")
	artifactHash := "hash-test"
	now := time.Now().UTC()

	meta := object.ArtifactMeta{
		Hash:      artifactHash,
		TaskRunID: taskRunID,
		CreatedAt: now,
	}

	if err := store.CreateArtifactMeta(ctx, meta); err != nil {
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
	taskRunID_1 := createTaskRun(t, store, ctx, "container-def-1")
	taskRunID_2 := createTaskRun(t, store, ctx, "container-def-2")
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

	if err := store.CreateArtifactMeta(ctx, meta_1); err != nil {
		t.Fatalf("Failed to save artifact meta_1: %v", err)
	}
	if err := store.CreateArtifactMeta(ctx, meta_2); err != nil {
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
	artifactHash := "hash-test"
	now := time.Now().UTC()

	meta := object.ArtifactMeta{
		Hash:      artifactHash,
		TaskRunID: "ghost-task",
		CreatedAt: now,
	}

	err := store.CreateArtifactMeta(ctx, meta)
	if err == nil {
		t.Fatal("Expected FOREIGN KEY constraint failure when inserting artifact without existing task")
	}
}

func TestArtifactMeta_DeleteTask(t *testing.T) {
	store := setupTestDB(t)
	defer store.Close()

	ctx := context.Background()
	taskRunID := createTaskRun(t, store, ctx, "container-def")
	artifactHash := "hash-test"
	now := time.Now().UTC()

	meta := object.ArtifactMeta{
		Hash:      artifactHash,
		TaskRunID: taskRunID,
		CreatedAt: now,
	}

	if err := store.CreateArtifactMeta(ctx, meta); err != nil {
		t.Fatalf("Failed to save artifact meta: %v", err)
	}

	if err := store.DeleteTaskRun(ctx, taskRunID); err != nil {
		t.Fatalf("Failed to delete parent task: %v", err)
	}

	metas, err := store.GetArtifactMeta(ctx, artifactHash)
	if err != nil {
		t.Fatalf("Failed to get artifact meta after task deletion: %v", err)
	}
	if len(metas) != 0 {
		t.Fatal("Expected artifact to be deleted via CASCADE, but it still exists")
	}
}

func TestGetNonExistentData(t *testing.T) {
	store := setupTestDB(t)
	defer store.Close()
	ctx := context.Background()

	if _, err := store.GetTaskRun(ctx, "ghost-task"); err == nil {
		t.Errorf("Expected error when fetching non-existent task run, got nil")
	}

	metas, err := store.GetArtifactMeta(ctx, "ghost-hash")
	if err != nil {
		t.Errorf("Expected no error when fetching non-existent artifact meta, got %v", err)
	}
	if len(metas) != 0 {
		t.Errorf("Expected no artifact meta entries, got %d", len(metas))
	}
}
