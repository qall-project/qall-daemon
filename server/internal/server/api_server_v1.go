package server

import (
	"context"
	"errors"
	"fmt"
	"log"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/qall-project/qall-daemon/server/internal/core"
	"github.com/qall-project/qall-daemon/server/internal/object"
	"github.com/qall-project/qall-daemon/server/internal/runtime"
	pbDaemon "github.com/qall-project/qall-daemon/server/internal/server/protobuf/daemon_runtime_api_v1"
)

// Machine-readable reasons attached to dataloader failures in an
// errdetails.ErrorInfo detail, so the SDK can branch on a code instead of
// parsing the message.
const (
	reasonDataloaderDependencyMissing = "QALL_DATALOADER_DEPENDENCY_MISSING"
	reasonDataloaderTimeout           = "QALL_DATALOADER_TIMEOUT"
	reasonDataloaderMountFailed       = "QALL_DATALOADER_MOUNT_FAILED"

	// dataloaderErrorDomain scopes every ErrorInfo produced by the daemon.
	dataloaderErrorDomain = "qall.daemon"
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
	log.Println("Request received: CreateWorkerEntry:", req.GetWorkerProvider(), req.GetInputFormat())

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
	log.Println("Request received: CreateResourceAssignment:", req.GetResourceProvider(), req.GetResourceName())

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

	if req.GetWorkflowRunId() == "" {
		return nil, status.Error(codes.InvalidArgument, "workflow_run_id cannot be empty: the daemon scopes each task's shared filesystem (/workdir) per workflow run")
	}

	dataloaders := make([]object.DataloaderEntry, 0, len(req.GetDataloaders()))
	for _, d := range req.GetDataloaders() {
		dataloaders = append(dataloaders, object.DataloaderEntry{
			TaskHash: d.GetTaskHash(),
			DataloaderSpec: object.DataloaderSpec{
				Name:        d.GetName(),
				Src:         d.GetSrc(),
				Dst:         d.GetDst(),
				Credentials: d.GetCredentials(),
			},
		})
	}

	taskRun, err := s.core.RunTask(ctx, req.GetTaskHash(), req.GetArtifactHash(), req.GetProviderCredentials(), req.GetWorkflowRunId(), dataloaders)

	if err != nil {
		if statusErr := dataloaderStatusError(err); statusErr != nil {
			return nil, statusErr
		}

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

// dataloaderStatusError translates a dataloader volume failure into a gRPC
// status error carrying an errdetails.ErrorInfo detail (reason + hint), so the
// SDK can branch on a stable reason code instead of parsing the message. It
// returns nil for any other error, leaving existing behaviour unchanged.
//
// The dataloader error message is already credential-free (the daemon redacts
// driver options before wrapping), so it is safe to expose to the client.
func dataloaderStatusError(err error) error {
	switch {
	case errors.Is(err, runtime.ErrDataloaderDependencyMissing):
		return dataloaderStatusErrorWithReason(err, codes.FailedPrecondition, reasonDataloaderDependencyMissing)
	case errors.Is(err, runtime.ErrDataloaderTimeout):
		return dataloaderStatusErrorWithReason(err, codes.Unavailable, reasonDataloaderTimeout)
	case errors.Is(err, runtime.ErrDataloaderMountFailed):
		return dataloaderStatusErrorWithReason(err, codes.Internal, reasonDataloaderMountFailed)
	default:
		return nil
	}
}

// dataloaderStatusErrorWithReason builds the status error and attaches the
// ErrorInfo detail. If attaching the detail fails (it cannot for a populated
// ErrorInfo), the plain status error is returned rather than losing the
// failure.
func dataloaderStatusErrorWithReason(err error, code codes.Code, reason string) error {
	hint := ""

	var mountErr *runtime.DataloaderMountError
	if errors.As(err, &mountErr) {
		hint = mountErr.Hint
	}

	st := status.New(code, err.Error())

	detailed, detailErr := st.WithDetails(&errdetails.ErrorInfo{
		Reason:   reason,
		Domain:   dataloaderErrorDomain,
		Metadata: map[string]string{"hint": hint},
	})
	if detailErr != nil {
		return st.Err()
	}

	return detailed.Err()
}
