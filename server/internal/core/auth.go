package core

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"google.golang.org/grpc/metadata"
)

var hostSecretsDir = "/dev/shm/qall-secrets"

func (e *DaemonCore) AuthTask(ctx context.Context, taskRunId string) (bool, error) {
	if taskRunId == "" {
		log.Printf("[Daemon] CreateArtifact with unidentified TaskRun ID")
		return false, nil
	}

	md, ok := metadata.FromIncomingContext(ctx)

	if !ok {
		log.Println("[Security Warning] CreateArtifact attempt without metadata")
		return false, nil
	}

	tokens := md["x-qall-token"]

	if len(tokens) == 0 || tokens[0] == "" {
		log.Printf("[Security Warning] CreateArtifact attempt without x-qall-token")
		return false, nil
	}

	clientToken := tokens[0]

	if err := e.verifyToken(taskRunId, clientToken); err != nil {
		log.Printf("[Security Alert] Unauthorized token '%s' - Error: %v", clientToken, err)
		return false, fmt.Errorf("unauthorized or expired execution token")
	}

	log.Printf("[Daemon] Authorized CreateArtifact for TaskRun ID: %s", taskRunId)

	return true, nil
}

func (e *DaemonCore) verifyToken(taskRunId string, token string) error {
	tokenPath := filepath.Join(hostSecretsDir, fmt.Sprintf("%s.token", taskRunId))

	data, err := os.ReadFile(tokenPath)

	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("invalid, unknown, or expired execution token (file not found)")
		}

		return fmt.Errorf("failed to read token file: %w", err)
	}

	if string(data) != token {
		return fmt.Errorf("invalid execution token (mismatch)")
	}

	return nil
}
