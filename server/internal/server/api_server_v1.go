package server

import (
	"context"
	"fmt"
	"log"

	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/qall-project/qall-daemon/server/core"
	"github.com/qall-project/qall-daemon/server/object"
	pbDaemon "github.com/qall-project/qall-daemon/server/server/protobuf/daemon_runtime_api_v1"
)

type ApiV1Server struct {
	pbDaemon.UnimplementedApiServer
	core *core.DaemonCore
}

func NewApiV1Server(dcore *core.DaemonCore) *ApiV1Server {
	return &ApiV1Server{core: dcore}
}

func (s *ApiV1Server) GetServiceInfo(ctx context.Context, in *emptypb.Empty) (*pbDaemon.DaemonInfo, error) {
	log.Println("Request received: GetServiceInfo")
	return &pbDaemon.DaemonInfo{Name: "qall-daemon", Version: "v1"}, nil
}

func (s *ApiV1Server) CreateWorkerEntry(ctx context.Context, req *pbDaemon.CreateWorkerEntryRequest) (*pbDaemon.WorkerEntry, error) {
	log.Println("Request received: CreateWorkerEntry")

	wk, err := s.core.CreateWorkerEntry(ctx, req.Provider, req.WorkerHash, req.InputFormat)

	if err != nil {
		return nil, err
	}

	return &pbDaemon.WorkerEntry{
		Provider:    wk.Provider,
		Hash:        wk.Hash,
		InputFormat: wk.InputFormat,
	}, nil
}

func (s *ApiV1Server) CreateTaskRun(ctx context.Context, req *pbDaemon.CreateTaskRunRequest) (*pbDaemon.TaskRun, error) {
	log.Println("Request received: CreateTaskRun")

	taskRun, err := s.core.RunTask(ctx, req.TaskHash, req.ArtifactHash)

	if err != nil {
		return nil, err
	}

	return &pbDaemon.TaskRun{
		Id:     taskRun.Id,
		Status: pbDaemon.TaskRun_PENDING,
	}, nil
}

func (s *ApiV1Server) GetTaskRun(ctx context.Context, req *pbDaemon.GetTaskRunRequest) (*pbDaemon.TaskRun, error) {
	log.Printf("Request received: GetTaskRun for ID: %s", req.TaskRunId)

	taskRun, err := s.core.GetTaskStatus(ctx, req.TaskRunId)
	if err != nil {
		return nil, err
	}

	var protoStatus pbDaemon.TaskRun_Status
	switch taskRun.Status {
	case object.StatusPending:
		protoStatus = pbDaemon.TaskRun_PENDING
	case object.StatusRunning:
		protoStatus = pbDaemon.TaskRun_RUNNING
	case object.StatusDone:
		protoStatus = pbDaemon.TaskRun_DONE
	case object.StatusError:
		protoStatus = pbDaemon.TaskRun_ERROR
	default:
		protoStatus = pbDaemon.TaskRun_UNKNOWN
	}

	return &pbDaemon.TaskRun{
		Id:     taskRun.Id,
		Status: protoStatus,
	}, nil
}

func (s *ApiV1Server) CreateArtifact(ctx context.Context, req *pbDaemon.CreateArtifactRequest) (*pbDaemon.Artifact, error) {
	log.Println("Request received: CreateArtifact")

	auth, err := s.core.AuthTask(ctx, req.TaskRunId)
	if err != nil {
		return nil, err
	}

	newArtifact := object.ArtifactRequest{
		TaskRunId: req.TaskRunId,
		Payload:   req.Payload,
	}

	info, err := s.core.CreateArtifact(ctx, newArtifact, auth)
	if err != nil {
		log.Printf("[Error] Failed to store artifact: %v", err)
		return nil, fmt.Errorf("failed to persist artifact: %v", err)
	}

	return &pbDaemon.Artifact{
		Hash: info.Hash,
	}, nil
}

func (s *ApiV1Server) DownloadArtifact(ctx context.Context, req *pbDaemon.DownloadArtifactRequest) (*pbDaemon.ArtifactPayload, error) {
	log.Println("Request received: DownloadArtifact")

	if req.ArtifactHash == "" {
		return nil, fmt.Errorf("artifact_hash is required")
	}

	payload, err := s.core.GetArtifact(ctx, req.ArtifactHash)
	if err != nil {
		log.Printf("[Error] DownloadArtifact failed for hash %s: %v", req.ArtifactHash, err)
		return nil, fmt.Errorf("failed to get artifact: %v", err)
	}

	return &pbDaemon.ArtifactPayload{
		Payload: payload,
	}, nil
}

func (s *ApiV1Server) ListArtifacts(ctx context.Context, req *pbDaemon.ListArtifactsRequest) (*pbDaemon.ListArtifactsResponse, error) {
	log.Println("Request received: ListArtifacts")

	if req.TaskRunId == "" {
		return nil, fmt.Errorf("task_hash is required")
	}

	artifacts, err := s.core.ListArtifacts(ctx, req.TaskRunId)
	if err != nil {
		log.Printf("[Error] ListArtifacts failed for task %s: %v", req.TaskRunId, err)
		return nil, fmt.Errorf("failed to list artifacts: %v", err)
	}

	pbArtifacts := make([]*pbDaemon.Artifact, len(artifacts))
	for i, art := range artifacts {
		pbArtifacts[i] = &pbDaemon.Artifact{
			Hash: art.Hash,
		}
	}

	return &pbDaemon.ListArtifactsResponse{
		Artifacts: pbArtifacts,
	}, nil
}
