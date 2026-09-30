package datastore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"

	_ "modernc.org/sqlite"

	"qall-daemon-server/internal/object"
)

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(rootDir string) (*SQLiteStore, error) {
	dbPath := filepath.Join(rootDir, "daemon_state.db")

	if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to remove existing SQLite database: %w", err)
	}

	db, err := sql.Open(
		"sqlite",
		dbPath+"?_pragma=foreign_keys(1)",
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to open sqlite db: %w",
			err,
		)
	}

	store := &SQLiteStore{
		db: db,
	}

	if err := store.migrate(context.Background()); err != nil {
		_ = db.Close()

		return nil, fmt.Errorf(
			"failed to migrate sqlite database: %w",
			err,
		)
	}

	return store, nil
}

func (s *SQLiteStore) Close() error {
	if s.db == nil {
		return nil
	}

	return s.db.Close()
}

func (s *SQLiteStore) migrate(ctx context.Context) error {
	schema := `
	CREATE TABLE IF NOT EXISTS task_runs (
		id TEXT PRIMARY KEY,
		container_id TEXT,
		status TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS workers (
		id TEXT PRIMARY KEY,
		task_run_id TEXT,
		container_id TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (task_run_id)
			REFERENCES task_runs(id)
			ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_workers_task_run_id
		ON workers(task_run_id);

	CREATE TABLE IF NOT EXISTS artifacts_meta (
		hash TEXT NOT NULL,
		task_run_id TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		PRIMARY KEY (hash, task_run_id),
		FOREIGN KEY (task_run_id)
			REFERENCES task_runs(id)
			ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_artifacts_task_id
		ON artifacts_meta(task_run_id);
	`

	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return err
	}

	return nil
}

func (s *SQLiteStore) CreateWorkerRun(
	ctx context.Context,
	workerRun object.WorkerRun,
) (string, error) {
	if workerRun.TaskRunId == "" {
		return "", errors.New("worker run task run ID cannot be empty")
	}

	workerRunID := uuid.New().String()

	query := `
		INSERT INTO workers (
			id,
			task_run_id,
			container_id
		)
		VALUES (?, ?, ?);
	`

	if _, err := s.db.ExecContext(
		ctx,
		query,
		workerRunID,
		workerRun.TaskRunId,
		workerRun.ContainerId,
	); err != nil {
		return "", err
	}

	return workerRunID, nil
}

func (s *SQLiteStore) GetWorkerRun(
	ctx context.Context,
	workerRunID string,
) (object.WorkerRun, error) {
	if workerRunID == "" {
		return object.WorkerRun{}, errors.New("worker run ID cannot be empty")
	}

	query := `SELECT id, task_run_id, container_id FROM workers WHERE id = ?;`
	var workerRun object.WorkerRun
	err := s.db.QueryRowContext(ctx, query, workerRunID).Scan(
		&workerRun.Id,
		&workerRun.TaskRunId,
		&workerRun.ContainerId,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return object.WorkerRun{}, fmt.Errorf("worker run %s not found", workerRunID)
	}

	if err != nil {
		return object.WorkerRun{}, err
	}

	return workerRun, nil
}

func (s *SQLiteStore) UpdateWorkerRun(
	ctx context.Context,
	workerRun object.WorkerRun,
) (object.WorkerRun, error) {
	if workerRun.Id == "" {
		return object.WorkerRun{}, errors.New(
			"worker run ID cannot be empty",
		)
	}

	if workerRun.TaskRunId == "" {
		return object.WorkerRun{}, errors.New(
			"worker run task run ID cannot be empty",
		)
	}

	if workerRun.ContainerId == "" {
		return object.WorkerRun{}, errors.New(
			"worker run container ID cannot be empty",
		)
	}

	query := `
		UPDATE workers
		SET
			task_run_id = ?,
			container_id = ?
		WHERE id = ?;
	`

	result, err := s.db.ExecContext(
		ctx,
		query,
		workerRun.TaskRunId,
		workerRun.ContainerId,
		workerRun.Id,
	)
	if err != nil {
		return object.WorkerRun{}, err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return object.WorkerRun{}, err
	}

	if rows == 0 {
		return object.WorkerRun{}, fmt.Errorf(
			"worker run %s not found",
			workerRun.Id,
		)
	}

	return s.GetWorkerRun(ctx, workerRun.Id)
}

func (s *SQLiteStore) DeleteWorkerRun(
	ctx context.Context,
	workerRunID string,
) error {
	if workerRunID == "" {
		return errors.New(
			"worker run ID cannot be empty",
		)
	}

	query := `
		DELETE FROM workers
		WHERE id = ?;
	`

	if _, err := s.db.ExecContext(
		ctx,
		query,
		workerRunID,
	); err != nil {
		return err
	}

	return nil
}

func (s *SQLiteStore) ListWorkerRuns(
	ctx context.Context,
) ([]object.WorkerRun, error) {
	query := `SELECT id, task_run_id, container_id FROM workers ORDER BY created_at ASC;`
	rows, err := s.db.QueryContext(ctx, query)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	workers := make([]object.WorkerRun, 0)

	for rows.Next() {
		var workerRun object.WorkerRun

		if err := rows.Scan(
			&workerRun.Id,
			&workerRun.TaskRunId,
			&workerRun.ContainerId,
		); err != nil {
			return nil, err
		}
		workers = append(workers, workerRun)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return workers, nil
}

func (s *SQLiteStore) ListWorkerRunsByTask(
	ctx context.Context,
	taskRunID string,
) ([]object.WorkerRun, error) {
	if taskRunID == "" {
		return nil, errors.New("task run ID cannot be empty")
	}

	query := `SELECT id, task_run_id, container_id FROM workers WHERE task_run_id = ? ORDER BY created_at ASC;`
	rows, err := s.db.QueryContext(ctx, query, taskRunID)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	workers := make([]object.WorkerRun, 0)

	for rows.Next() {
		var workerRun object.WorkerRun

		if err := rows.Scan(
			&workerRun.Id,
			&workerRun.TaskRunId,
			&workerRun.ContainerId,
		); err != nil {
			return nil, err
		}
		workers = append(workers, workerRun)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return workers, nil
}

func (s *SQLiteStore) CreateTaskRun(
	ctx context.Context,
	taskRun object.TaskRun,
) (string, error) {
	taskRunID := uuid.New().String()

	query := `
		INSERT INTO task_runs (
			id,
			container_id,
			status
		)
		VALUES (?, ?, ?);
	`

	if _, err := s.db.ExecContext(
		ctx,
		query,
		taskRunID,
		taskRun.ContainerId,
		taskRun.Status,
	); err != nil {
		return "", err
	}

	return taskRunID, nil
}

func (s *SQLiteStore) GetTaskRun(
	ctx context.Context,
	taskRunID string,
) (object.TaskRun, error) {
	if taskRunID == "" {
		return object.TaskRun{}, errors.New("task run ID cannot be empty")
	}

	query := `SELECT id, container_id, status FROM task_runs WHERE id = ?;`
	var taskRun object.TaskRun
	err := s.db.QueryRowContext(ctx, query, taskRunID).Scan(
		&taskRun.Id,
		&taskRun.ContainerId,
		&taskRun.Status,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return object.TaskRun{}, fmt.Errorf("task run %s not found", taskRunID)
	}

	if err != nil {
		return object.TaskRun{}, err
	}

	return taskRun, nil
}

func (s *SQLiteStore) UpdateTaskRun(
	ctx context.Context,
	taskRun object.TaskRun,
) (object.TaskRun, error) {
	if taskRun.Id == "" {
		return object.TaskRun{}, errors.New(
			"task run ID cannot be empty",
		)
	}

	query := `
		UPDATE task_runs
		SET
			container_id = ?,
			status = ?
		WHERE id = ?;
	`

	result, err := s.db.ExecContext(
		ctx,
		query,
		taskRun.ContainerId,
		taskRun.Status,
		taskRun.Id,
	)

	if err != nil {
		return object.TaskRun{}, err
	}

	rows, err := result.RowsAffected()

	if err != nil {
		return object.TaskRun{}, err
	}

	if rows == 0 {
		return object.TaskRun{}, fmt.Errorf(
			"task run %s not found",
			taskRun.Id,
		)
	}

	return s.GetTaskRun(ctx, taskRun.Id)
}

func (s *SQLiteStore) DeleteTaskRun(
	ctx context.Context,
	taskRunID string,
) error {
	if taskRunID == "" {
		return errors.New(
			"task run ID cannot be empty",
		)
	}

	query := `
		DELETE FROM task_runs
		WHERE id = ?;
	`

	if _, err := s.db.ExecContext(
		ctx,
		query,
		taskRunID,
	); err != nil {
		return err
	}

	return nil
}

func (s *SQLiteStore) CreateArtifactMeta(
	ctx context.Context,
	meta object.ArtifactMeta,
) error {
	if meta.Hash == "" {
		return errors.New(
			"artifact hash cannot be empty",
		)
	}

	if meta.TaskRunID == "" {
		return errors.New(
			"artifact task run ID cannot be empty",
		)
	}

	query := `
		INSERT INTO artifacts_meta (
			hash,
			task_run_id,
			created_at
		)
		VALUES (?, ?, ?)
		ON CONFLICT(hash, task_run_id) DO NOTHING;
	`

	if _, err := s.db.ExecContext(
		ctx,
		query,
		meta.Hash,
		meta.TaskRunID,
		meta.CreatedAt,
	); err != nil {
		return err
	}

	return nil
}

func (s *SQLiteStore) GetArtifactMeta(
	ctx context.Context,
	hash string,
) ([]object.ArtifactMeta, error) {
	if hash == "" {
		return nil, errors.New(
			"artifact hash cannot be empty",
		)
	}

	query := `
		SELECT
			hash,
			task_run_id,
			created_at
		FROM artifacts_meta
		WHERE hash = ?
		ORDER BY created_at ASC;
	`

	rows, err := s.db.QueryContext(
		ctx,
		query,
		hash,
	)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	results := make([]object.ArtifactMeta, 0)

	for rows.Next() {
		var meta object.ArtifactMeta

		if err := rows.Scan(
			&meta.Hash,
			&meta.TaskRunID,
			&meta.CreatedAt,
		); err != nil {
			return nil, err
		}

		results = append(results, meta)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return results, nil
}

func (s *SQLiteStore) ListArtifactsByTask(
	ctx context.Context,
	taskRunID string,
) ([]object.ArtifactMeta, error) {
	if taskRunID == "" {
		return nil, errors.New(
			"task run ID cannot be empty",
		)
	}

	query := `
		SELECT
			hash,
			task_run_id,
			created_at
		FROM artifacts_meta
		WHERE task_run_id = ?
		ORDER BY created_at ASC;
	`

	rows, err := s.db.QueryContext(
		ctx,
		query,
		taskRunID,
	)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	results := make([]object.ArtifactMeta, 0)

	for rows.Next() {
		var meta object.ArtifactMeta

		if err := rows.Scan(
			&meta.Hash,
			&meta.TaskRunID,
			&meta.CreatedAt,
		); err != nil {
			return nil, err
		}

		results = append(results, meta)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return results, nil
}
