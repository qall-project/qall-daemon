package core

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyToken(t *testing.T) {
	originalDir := hostSecretsDir
	hostSecretsDir = t.TempDir()
	defer func() { hostSecretsDir = originalDir }()

	dcore := &DaemonCore{}

	validTaskRunId := "task-1234"
	validToken := "super-secret-token"

	tokenPath := filepath.Join(hostSecretsDir, fmt.Sprintf("%s.token", validTaskRunId))
	err := os.WriteFile(tokenPath, []byte(validToken), 0600)
	if err != nil {
		t.Fatalf("Failed to setup mock token file: %v", err)
	}

	tests := []struct {
		name        string
		taskRunId   string
		inputToken  string
		expectError bool
	}{
		{
			name:        "Valid token matches",
			taskRunId:   validTaskRunId,
			inputToken:  validToken,
			expectError: false,
		},
		{
			name:        "Invalid token mismatch",
			taskRunId:   validTaskRunId,
			inputToken:  "wrong-token",
			expectError: true,
		},
		{
			name:        "Missing token file (unknown task)",
			taskRunId:   "unknown-task",
			inputToken:  "any-token",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := dcore.verifyToken(tt.taskRunId, tt.inputToken)

			if tt.expectError && err == nil {
				t.Errorf("Expected an error but got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}
