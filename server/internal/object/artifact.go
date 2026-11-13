package object

import "time"

type CreateArtifactRequest struct {
	TaskRunId string
	Payload   []byte
}

type Artifact struct {
	Hash string
}

type ArtifactMeta struct {
	Hash      string    `json:"hash"`
	TaskRunID string    `json:"task_run_id"`
	CreatedAt time.Time `json:"created_at"`
}
