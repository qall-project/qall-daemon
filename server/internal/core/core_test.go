package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"

	dsObj "github.com/qall-project/qall-daemon/server/internal/datastore/object"
	"github.com/qall-project/qall-daemon/server/internal/object"
)

type mockBlockStore struct {
	blocks map[string][]byte
	hasErr error
	putErr error
	getErr error
}

func newMockBlockStore() *mockBlockStore {
	return &mockBlockStore{
		blocks: make(map[string][]byte),
	}
}

func (m *mockBlockStore) Has(ctx context.Context, hash string) (bool, error) {
	if m.hasErr != nil {
		return false, m.hasErr
	}
	_, exists := m.blocks[hash]
	return exists, nil
}

func (m *mockBlockStore) Put(ctx context.Context, hash string, data []byte) error {
	if m.putErr != nil {
		return m.putErr
	}
	m.blocks[hash] = data
	return nil
}

func (m *mockBlockStore) Get(ctx context.Context, hash string) ([]byte, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	data, exists := m.blocks[hash]
	if !exists {
		return nil, fmt.Errorf("block not found")
	}
	return data, nil
}

type mockMetaStore struct {
	dsObj.MetadataStore
	savedMetas []dsObj.ArtifactMeta
	listMetas  []dsObj.ArtifactMeta
	saveErr    error
	listErr    error
}

func (m *mockMetaStore) SaveArtifactMeta(ctx context.Context, meta dsObj.ArtifactMeta) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.savedMetas = append(m.savedMetas, meta)
	return nil
}

func (m *mockMetaStore) ListArtifactsByTask(ctx context.Context, taskRunID string) ([]dsObj.ArtifactMeta, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}

	var filtered []dsObj.ArtifactMeta
	for _, meta := range m.listMetas {
		if meta.TaskRunID == taskRunID {
			filtered = append(filtered, meta)
		}
	}
	return filtered, nil
}

type mockContainerRuntime struct{}

func (m *mockContainerRuntime) EnsureImage(ctx context.Context, image string, requirements string) (string, error) {
	return "mock-image:latest", nil
}

func (m *mockContainerRuntime) StartWorker(ctx context.Context, image string, args object.ExecuteArgs) (string, error) {
	return "mock-container-123", nil
}

func (m *mockContainerRuntime) GetWorkerStatus(ctx context.Context, containerID string) (object.TaskStatus, error) {
	return object.StatusDone, nil
}

func (m *mockContainerRuntime) Close() error {
	return nil
}

func TestEngine_RunTask_TaskHashEmpty(t *testing.T) {
	engine := NewDaemonCore(nil, "/fake/dir", nil, nil, &mockContainerRuntime{})
	ctx := context.Background()

	_, err := engine.RunTask(ctx, "", "artifact-hash")
	if err == nil || err.Error() != "TaskHash can't be empty" {
		t.Fatalf("Expected 'TaskHash can't be empty' error, got: %v", err)
	}
}

func TestEngine_CreateArtifact(t *testing.T) {
	ctx := context.Background()

	artifact := []byte("hello world")
	hasher := sha256.New()
	hasher.Write(artifact)
	expectedHash := hex.EncodeToString(hasher.Sum(nil))

	tests := []struct {
		name         string
		payload      []byte
		taskRunId    string
		auth         bool
		savedMeta    int
		expectErr    bool
		expectErrMsg string
	}{
		{
			name:         "Empty payload should fail",
			payload:      nil,
			taskRunId:    "task-abc",
			auth:         true,
			savedMeta:    0,
			expectErr:    true,
			expectErrMsg: "artifact payload cannot be empty",
		},
		{
			name:         "Valid payload without auth",
			payload:      artifact,
			taskRunId:    "task-def",
			auth:         false,
			savedMeta:    0,
			expectErr:    false,
			expectErrMsg: "",
		},
		{
			name:         "Valid payload with auth",
			payload:      artifact,
			taskRunId:    "task-ghi",
			auth:         true,
			savedMeta:    1,
			expectErr:    false,
			expectErrMsg: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockBS := newMockBlockStore()
			mockMeta := &mockMetaStore{}
			engine := NewDaemonCore(nil, "", mockMeta, mockBS, &mockContainerRuntime{})

			req := object.ArtifactRequest{Payload: tt.payload, TaskRunId: tt.taskRunId}
			artifact, err := engine.CreateArtifact(ctx, req, tt.auth)

			if tt.expectErr {
				if err == nil || err.Error() != tt.expectErrMsg {
					t.Errorf("Expected an error (%s) but got: %v", tt.expectErrMsg, err)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if artifact.Hash != expectedHash {
				t.Errorf("Expected hash %s, got %s", expectedHash, artifact.Hash)
			}

			if string(mockBS.blocks[expectedHash]) != string(tt.payload) {
				t.Errorf("Payload not saved correctly to block store")
			}

			if len(mockMeta.savedMetas) != tt.savedMeta {
				t.Fatalf("Expected %d metadata to be saved, got %d entries", tt.savedMeta, len(mockMeta.savedMetas))
			}

			if tt.savedMeta >= 1 && mockMeta.savedMetas[0].TaskRunID != tt.taskRunId {
				t.Errorf("Expected metadata linked to 'task-2', got %s", mockMeta.savedMetas[0].TaskRunID)
			}
		})
	}
}

func TestEngine_GetArtifact(t *testing.T) {
	mockBS := newMockBlockStore()
	engine := NewDaemonCore(nil, "", nil, mockBS, &mockContainerRuntime{})
	ctx := context.Background()

	hash := "some-hash"
	data := []byte("artifact data")
	mockBS.blocks[hash] = data

	tests := []struct {
		name         string
		hash         string
		expectErr    bool
		expectErrMsg string
	}{
		{
			name:         "Empty hash should fail",
			hash:         "",
			expectErr:    true,
			expectErrMsg: "artifact hash cannot be empty",
		},
		{
			name:         "Valid hash should return data",
			hash:         hash,
			expectErr:    false,
			expectErrMsg: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			retrieved, err := engine.GetArtifact(ctx, tt.hash)
			if tt.expectErr {
				if err == nil || err.Error() != tt.expectErrMsg {
					t.Errorf("Expected an error (%s) but got: %v", tt.expectErrMsg, err)
				}
				return
			}
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if !bytes.Equal(retrieved, data) {
				t.Errorf("Expected %s, got %s", data, retrieved)
			}
		})
	}
}

func TestEngine_ListArtifacts(t *testing.T) {
	mockMeta := &mockMetaStore{}
	engine := NewDaemonCore(nil, "", mockMeta, nil, &mockContainerRuntime{})
	ctx := context.Background()

	mockMeta.listMetas = []dsObj.ArtifactMeta{
		{Hash: "hash-1", TaskRunID: "task-abc"},
		{Hash: "hash-2", TaskRunID: "task-abc"},
	}

	tests := []struct {
		name           string
		taskRunId      string
		expectArtifact int
		expectErr      bool
		expectErrMsg   string
	}{
		{
			name:           "Empty task run ID should fail",
			taskRunId:      "",
			expectArtifact: 0,
			expectErr:      true,
			expectErrMsg:   "task run ID cannot be empty",
		},
		{
			name:           "Valid task run ID should return artifacts",
			taskRunId:      "task-abc",
			expectArtifact: 2,
			expectErr:      false,
			expectErrMsg:   "",
		},
		{
			name:           "Valid task run ID with no artifacts",
			taskRunId:      "task-def",
			expectArtifact: 0,
			expectErr:      false,
			expectErrMsg:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			artifacts, err := engine.ListArtifacts(ctx, tt.taskRunId)

			if tt.expectErr {
				if err == nil || err.Error() != tt.expectErrMsg {
					t.Errorf("Expected an error (%s) but got: %v", tt.expectErrMsg, err)
				}
				return
			}
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if len(artifacts) != tt.expectArtifact {
				t.Fatalf("Expected %d artifacts, got %d", tt.expectArtifact, len(artifacts))
			}
			if tt.expectArtifact > 0 && (artifacts[0].Hash != "hash-1" || artifacts[1].Hash != "hash-2") {
				t.Errorf("Returned artifacts mismatch")
			}
		})
	}
}
