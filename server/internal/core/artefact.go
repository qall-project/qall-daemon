package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"qall-daemon-server/internal/object"
	"time"
)

func (e *DaemonCore) CreateArtifact(ctx context.Context, req object.ArtifactRequest, auth bool) (*object.Artifact, error) {
	if len(req.Payload) == 0 {
		return nil, fmt.Errorf("artifact payload cannot be empty")
	}

	hasher := sha256.New()
	hasher.Write(req.Payload)
	artifactHash := hex.EncodeToString(hasher.Sum(nil))

	exists, err := e.artifactStore.Has(ctx, artifactHash)

	if err != nil {
		return nil, fmt.Errorf("failed to check artifact storage: %w", err)
	}

	if !exists {
		log.Printf("[Core] Adding artifact to registry")

		if err := e.artifactStore.Put(ctx, artifactHash, req.Payload); err != nil {
			return nil, fmt.Errorf("failed to write artifact payload to CAS: %w", err)
		}
	} else {
		log.Printf("[Core] Artifact already present")
	}

	if auth {
		meta := object.ArtifactMeta{
			Hash:      artifactHash,
			TaskRunID: req.TaskRunId,
			CreatedAt: time.Now().UTC(),
		}

		if err := e.taskStore.CreateArtifactMeta(ctx, meta); err != nil {
			return nil, fmt.Errorf("failed to link artifact metadata: %w", err)
		}

		log.Printf("[Core] Persisting artifact (%d bytes) for TaskRun '%s'", len(req.Payload), req.TaskRunId[:8])
	} else {
		log.Printf("[Core] Persisting artifact (%d bytes)", len(req.Payload))
	}

	return &object.Artifact{
		Hash: artifactHash,
	}, nil
}

func (e *DaemonCore) GetArtifact(ctx context.Context, hash string) ([]byte, error) {
	if hash == "" {
		return nil, fmt.Errorf("artifact hash cannot be empty")
	}

	log.Printf("[Core] Fetching artifact block for hash: %s", hash)

	data, err := e.artifactStore.Get(ctx, hash)

	if err != nil {
		return nil, fmt.Errorf("failed to retrieve artifact block: %w", err)
	}

	return data, nil
}

func (e *DaemonCore) ListArtifacts(ctx context.Context, taskRunID string) ([]object.Artifact, error) {
	if taskRunID == "" {
		return nil, fmt.Errorf("task run ID cannot be empty")
	}

	log.Printf("[Core] Listing artifacts for TaskRun: %s", taskRunID)

	metas, err := e.taskStore.ListArtifactsByTask(ctx, taskRunID)

	if err != nil {
		return nil, fmt.Errorf("failed to list artifacts from metadata store: %w", err)
	}

	artifacts := make([]object.Artifact, len(metas))

	for i, m := range metas {
		artifacts[i] = object.Artifact{
			Hash: m.Hash,
		}
	}

	return artifacts, nil
}
