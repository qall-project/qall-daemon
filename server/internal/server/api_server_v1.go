package server

import (
	"context"
	"fmt"
	"log"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/qall-project/qall-daemon/server/internal/core"
	"github.com/qall-project/qall-daemon/server/internal/object"
	pbDaemon "github.com/qall-project/qall-daemon/server/internal/server/protobuf/daemon_runtime_api_v1"
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

	wk, err := s.core.CreateWorkerEntry(ctx, req.GetWorkerProvider(), req.GetWorkerHash(), req.GetInputFormat())

	if err != nil {
		return nil, err
	}

	return &pbDaemon.WorkerEntry{
		WorkerProvider: wk.WorkerProvider,
		WorkerHash:     wk.WorkerHash,
		InputFormat:    wk.InputFormat,
	}, nil
}

func (s *ApiV1Server) CreateResourceAssignment(ctx context.Context, req *pbDaemon.CreateResourceAssignmentRequest) (*pbDaemon.ResourceAssignment, error) {
	log.Println("Request received: CreateResourceAssignment")

	ra, err := s.core.CreateResourceAssignment(ctx, req.GetResourceProvider(), req.GetTaskHash(), req.GetResourceName())

	if err != nil {
		return nil, err
	}

	return &pbDaemon.ResourceAssignment{
		ResourceProvider: ra.ResourceProvider,
		TaskHash:         ra.TaskHash,
		ResourceName:     ra.ResourceName,
	}, nil
}

func (s *ApiV1Server) CreateTaskRun(ctx context.Context, req *pbDaemon.CreateTaskRunRequest) (*pbDaemon.TaskRun, error) {
	log.Println("Request received: CreateTaskRun")

	if req.GetTaskHash() == "" {
		return nil, status.Error(codes.InvalidArgument, "task_hash cannot be empty")
	}

	taskRun, err := s.core.RunTask(ctx, req.GetTaskHash(), req.GetArtifactHash(), req.GetProviderCredentials())

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

	if req.GetTaskRunId() == "" {
		return nil, status.Error(codes.InvalidArgument, "task_run_id cannot be empty")
	}

	taskRun, err := s.core.GetTaskStatus(ctx, req.GetTaskRunId())
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

	auth, err := s.core.AuthTask(ctx, req.GetTaskRunId())
	if err != nil {
		return nil, err
	}

	newArtifact := object.ArtifactRequest{
		TaskRunId: req.GetTaskRunId(),
		Payload:   req.GetPayload(),
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

	if req.GetArtifactHash() == "" {
		return nil, fmt.Errorf("artifact_hash is required")
	}

	payload, err := s.core.GetArtifact(ctx, req.GetArtifactHash())
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

	if req.GetTaskRunId() == "" {
		return nil, fmt.Errorf("task_run_id is required")
	}

	artifacts, err := s.core.ListArtifacts(ctx, req.GetTaskRunId())
	if err != nil {
		log.Printf("[Error] ListArtifacts failed for task %s: %v", req.GetTaskRunId(), err)
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
