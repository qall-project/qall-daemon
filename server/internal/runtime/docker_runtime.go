package runtime

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/build"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"

	"github.com/qall-project/qall-daemon/server/internal/object"
)

type DockerRuntime struct {
	client            *client.Client
	daemonContainerID string
	networkName       string
}

const qallBaseImage = "rg.fr-par.scw.cloud/qall-project/qall-base:local"
const HostSecretsDir = "/dev/shm/qall-secrets"
const HostSandboxDir = "/tmp/qall_unifs"

func NewDockerRuntime(ctx context.Context) (*DockerRuntime, error) {
	cli, err := client.New(client.FromEnv)

	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	networkName := "qall-network"

	networks, err := cli.NetworkList(ctx, client.NetworkListOptions{})

	if err != nil {
		cli.Close()
		return nil, fmt.Errorf("error while listing networks: %w", err)
	}

	var networkID string

	for _, net := range networks.Items {
		if net.Name == networkName {
			networkID = net.ID
			break
		}
	}

	if networkID == "" {
		log.Printf("[Network] Build a docker network %s...\n", networkName)
		resp, err := cli.NetworkCreate(ctx, networkName, client.NetworkCreateOptions{
			Driver: "bridge",
		})

		if err != nil {
			cli.Close()
			return nil, fmt.Errorf("error while building a docker network: %w", err)
		}

		networkID = resp.ID
	} else {
		log.Printf("docker network %s is already up.\n", networkName)
	}

	daemonContainerID, err := os.Hostname()

	if err != nil {
		cli.Close()
		return nil, fmt.Errorf("couldn't find hostname: %w", err)
	}

	log.Printf("[Network] Connecting daemon (ID: %s) to the network %s...\n", daemonContainerID, networkName)
	_, err = cli.NetworkConnect(ctx, networkID, client.NetworkConnectOptions{
		Container:      daemonContainerID,
		EndpointConfig: &network.EndpointSettings{},
	})

	if err != nil && !strings.Contains(err.Error(), "already exists") {
		cli.Close()
		return nil, fmt.Errorf("error while connecting: %w", err)
	}

	return &DockerRuntime{
		client:            cli,
		daemonContainerID: daemonContainerID,
		networkName:       networkName,
	}, nil
}

func (d *DockerRuntime) CreateImage(
	ctx context.Context,
	targetImageName string,
	baseImageName string,
	qallVersion string,
	requirements []string,
) (string, error) {
	hashInput := fmt.Sprintf(
		"%s|qall:%s|%s",
		baseImageName,
		qallVersion,
		strings.Join(requirements, "|"),
	)

	hash := sha256.Sum256([]byte(hashInput))
	imageTag := fmt.Sprintf(
		"%s:%s",
		targetImageName,
		hex.EncodeToString(hash[:])[:16],
	)

	if _, err := d.client.ImageInspect(ctx, imageTag); err == nil {
		log.Printf(
			"[Docker-Build] Image %s already exists. Skipping build.",
			imageTag,
		)
		return imageTag, nil
	}

	log.Printf(
		"[Docker-Build] Building new image %s with base %s, qall %s and deps: %s",
		imageTag,
		baseImageName,
		qallVersion,
		requirements,
	)

	var dockerfile strings.Builder

	dockerfile.WriteString(fmt.Sprintf(
		"FROM %s\n",
		baseImageName,
	))
	dockerfile.WriteString("WORKDIR /qall-workspace\n")

	// Install Qall from PyPI.
	dockerfile.WriteString(
		"RUN --mount=type=cache,target=/root/.cache/pip " +
			fmt.Sprintf(
				"pip install --cache-dir=/root/.cache/pip qall==%s\n",
				qallVersion,
			),
	)

	// Install task/worker-specific requirements.
	if len(requirements) > 0 {
		formattedReqs := make([]string, len(requirements))

		for i, req := range requirements {
			formattedReqs[i] = fmt.Sprintf("%q", req)
		}

		dockerfile.WriteString(fmt.Sprintf(
			"RUN --mount=type=cache,target=/root/.cache/pip "+
				"pip install --cache-dir=/root/.cache/pip %s\n",
			strings.Join(formattedReqs, " "),
		))
	}

	buildCtx, err := createTarStream("Dockerfile", dockerfile.String())
	if err != nil {
		return "", fmt.Errorf("failed to create build context: %w", err)
	}

	resp, err := d.client.ImageBuild(ctx, buildCtx, client.ImageBuildOptions{
		Tags:    []string{imageTag},
		Remove:  true,
		Version: build.BuilderBuildKit,
	})

	if err != nil {
		return "", fmt.Errorf("failed to trigger image build: %w", err)
	}

	defer resp.Body.Close()

	var buildLog bytes.Buffer
	if _, err := io.Copy(&buildLog, resp.Body); err != nil {
		return "", fmt.Errorf("failed to read build response: %w", err)
	}

	if strings.Contains(buildLog.String(), `"error":`) {
		return "", fmt.Errorf(
			"docker build failed for image %s:\n%s",
			imageTag,
			buildLog.String(),
		)
	}

	log.Printf(
		"[Docker-Build] Image %s built successfully.",
		imageTag,
	)

	return imageTag, nil
}

func (d *DockerRuntime) StartTask(ctx context.Context, imageTag string, args object.TaskRuntimeArgs) (string, error) {
	if err := os.MkdirAll(HostSecretsDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create secrets directory: %w", err)
	}

	tokenFilename := fmt.Sprintf("%s.token", args.TaskRunId)
	tokenPath := filepath.Join(HostSecretsDir, tokenFilename)

	if err := os.WriteFile(tokenPath, []byte(args.Token), 0400); err != nil {
		return "", fmt.Errorf("failed to write token to RAM: %w", err)
	}

	workerRegistryDir := "/registry"
	workerTokenPath := "/run/secrets/qall_token"
	workerSandboxDir := "/unifs"
	mounts := []mount.Mount{
		{
			Type:     mount.TypeBind,
			Source:   args.HostBlockRegistryDir,
			Target:   workerRegistryDir,
			ReadOnly: true,
		},
		{
			Type:     mount.TypeBind,
			Source:   tokenPath,
			Target:   workerTokenPath,
			ReadOnly: true,
		},
		{
			Type:     mount.TypeBind,
			Source:   HostSandboxDir,
			Target:   workerSandboxDir,
			ReadOnly: false, // Explicitly false for read-write access
		},
	}

	cmd := []string{
		"uv", "run", "qall", "task", "run",
		args.TaskHash, args.TaskRunId,
		"--block-registry", workerRegistryDir,
	}

	if args.ArtifactHash != "" {
		cmd = append(cmd, "--artifact-hash", args.ArtifactHash)
	}

	for _, wa := range args.WorkerAddresses {
		cmd = append(cmd, "--worker-address", wa)
	}

	envVariables := []string{
		"PYTHONUNBUFFERED=1",
		fmt.Sprintf("QALL_TOKEN_PATH=%s", workerTokenPath),
		fmt.Sprintf("QALL_DAEMON_ADDRESS=%s:50053", d.daemonContainerID),
	}

	if args.EnvironmentVariables != nil {
		for key, value := range args.EnvironmentVariables {
			envVariables = append(envVariables, fmt.Sprintf("%s=%s", key, value))
		}
	}

	resp, err := d.client.ContainerCreate(ctx, client.ContainerCreateOptions{
		Name: args.Name,
		Config: &container.Config{
			Image: imageTag,
			Env:   envVariables,
			Cmd:   cmd,
		},
		HostConfig: &container.HostConfig{
			Mounts:      mounts,
			NetworkMode: container.NetworkMode(d.networkName),
		},
	})

	if err != nil {
		os.Remove(tokenPath)
		return "", fmt.Errorf("failed to create task container: %w", err)
	}

	if _, err := d.client.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{}); err != nil {
		os.Remove(tokenPath)
		return "", fmt.Errorf("failed to start task container: %w", err)
	}

	log.Printf("[Docker-Task] Container %s started for TaskRun %s", resp.ID[:8], args.TaskRunId)

	return resp.ID, nil
}

func (d *DockerRuntime) StartWorker(ctx context.Context, imageTag string, args object.WorkerRuntimeArgs) (string, error) {
	if err := os.MkdirAll(HostSecretsDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create secrets directory: %w", err)
	}

	tokenFilename := fmt.Sprintf("%s.token", args.WorkerRunId)
	tokenPath := filepath.Join(HostSecretsDir, tokenFilename)

	if err := os.WriteFile(tokenPath, []byte(args.Token), 0400); err != nil {
		return "", fmt.Errorf("failed to write token to RAM: %w", err)
	}

	workerRegistryDir := "/registry"
	workerTokenPath := "/run/secrets/qall_token"
	workerSandboxDir := "~/.cache/qall/unifs"

	mounts := []mount.Mount{
		{
			Type:     mount.TypeBind,
			Source:   args.HostBlockRegistryDir,
			Target:   workerRegistryDir,
			ReadOnly: true,
		},
		{
			Type:     mount.TypeBind,
			Source:   tokenPath,
			Target:   workerTokenPath,
			ReadOnly: true,
		},
		{
			Type:     mount.TypeBind,
			Source:   HostSandboxDir,
			Target:   workerSandboxDir,
			ReadOnly: false,
		},
	}

	cmd := []string{
		"uv", "run", "qall", "worker", "run",
		args.WorkerHash,
		"--block-registry", workerRegistryDir,
		"--port", args.Port,
	}

	containerName := args.Name

	envVariables := []string{
		"PYTHONUNBUFFERED=1",
		fmt.Sprintf("TASK_RUN_ID=%s", args.TaskRunId),
		fmt.Sprintf("QALL_TOKEN_PATH=%s", workerTokenPath),
		fmt.Sprintf("QALL_DAEMON_ADDRESS=%s:50053", d.daemonContainerID),
	}

	if args.EnvironmentVariables != nil {
		for key, value := range args.EnvironmentVariables {
			envVariables = append(envVariables, fmt.Sprintf("%s=%s", key, value))
		}
	}

	resp, err := d.client.ContainerCreate(ctx, client.ContainerCreateOptions{
		Name: containerName,
		Config: &container.Config{
			Image: imageTag,
			Env:   envVariables,
			Cmd:   cmd,
		},
		HostConfig: &container.HostConfig{
			Mounts:      mounts,
			NetworkMode: container.NetworkMode(d.networkName),
		},
	})

	if err != nil {
		// Clean up token if container creation fails
		os.Remove(tokenPath)
		return "", fmt.Errorf("failed to create worker container: %w", err)
	}

	if _, err := d.client.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{}); err != nil {
		// Clean up token if container startup fails
		os.Remove(tokenPath)
		return "", fmt.Errorf("failed to start worker container: %w", err)
	}

	log.Printf("[Docker-Worker] Container %s started for TaskRun %s", resp.ID[:8], args.TaskRunId)

	return resp.ID, nil
}

func (d *DockerRuntime) GetStatus(ctx context.Context, containerID string) (object.RuntimeStatus, error) {
	inspect, err := d.client.ContainerInspect(ctx, containerID, client.ContainerInspectOptions{})

	if err != nil {
		if errdefs.IsNotFound(err) {
			return object.StatusUnknown, nil
		}

		return "", err
	}

	state := inspect.Container.State

	if state == nil {
		return object.StatusUnknown, fmt.Errorf("could not retrieve container state")
	}

	switch state.Status {
	case "running", "restarting":
		return object.StatusRunning, nil
	case "created", "paused":
		return object.StatusPending, nil
	case "exited":
		if state.ExitCode == 0 {
			return object.StatusDone, nil
		}
		return object.StatusError, nil
	default:
		return object.StatusError, nil
	}
}

func (r *DockerRuntime) Stop(
	ctx context.Context,
	containerID string,
) error {
	timeout := 10

	if _, err := r.client.ContainerStop(
		ctx,
		containerID,
		client.ContainerStopOptions{
			Timeout: &timeout,
		},
	); err != nil {
		if errdefs.IsNotFound(err) {
			return nil
		}

		return err
	}

	return nil
}

func (d *DockerRuntime) Close() error {
	return d.client.Close()
}

func createTarStream(filename string, content string) (io.Reader, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	hdr := &tar.Header{
		Name:    filename,
		Mode:    0644,
		Size:    int64(len(content)),
		ModTime: time.Now(),
	}

	if err := tw.WriteHeader(hdr); err != nil {
		return nil, err
	}

	if _, err := tw.Write([]byte(content)); err != nil {
		return nil, err
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}

	return &buf, nil
}
