package runtime

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/moby/moby/api/types/build"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"golang.org/x/sync/singleflight"

	"github.com/qall-project/qall-daemon/server/internal/object"
)

type DockerRuntime struct {
	client            *client.Client
	daemonContainerID string
	networkName       string
	buildGroup        singleflight.Group
}

const HostSecretsDir = "/dev/shm/qall-secrets"
const HostDataloadersDir = "/dev/shm/qall-dataloaders"
const HostSandboxDir = "/tmp/qall/unifs"

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
	qallSource := strings.TrimSpace(os.Getenv("QALL_DEV_DIR"))

	hashInput := fmt.Sprintf(
		"%s|qall:%s|%s",
		baseImageName,
		qallVersion,
		strings.Join(requirements, "|"),
	)

	// When a local qall SDK dev dir is configured, its content becomes part of
	// the image cache key so that any local change triggers a rebuild instead
	// of silently reusing a stale image built from the previous source.
	if qallSource != "" {
		fingerprint, err := hashDirContent(qallSource)
		if err != nil {
			return "", fmt.Errorf("failed to fingerprint qall dev dir %q: %w", qallSource, err)
		}
		hashInput = fmt.Sprintf("%s|dev:%x", hashInput, fingerprint)
	}

	hash := sha256.Sum256([]byte(hashInput))
	imageTag := fmt.Sprintf(
		"%s:%s",
		targetImageName,
		hex.EncodeToString(hash[:])[:16],
	)

	// singleflight ensures only one execution runs for the same imageTag at a time
	_, err, _ := d.buildGroup.Do(imageTag, func() (interface{}, error) {
		if _, err := d.client.ImageInspect(ctx, imageTag); err == nil {
			log.Printf(
				"[Docker-Build] Image %s already exists. Skipping build.",
				imageTag,
			)
			return nil, nil
		}

		log.Printf(
			"[Docker-Build] Building new image %s with base %s, qall %s and deps: %s",
			imageTag,
			baseImageName,
			qallVersion,
			requirements,
		)

		var dockerfile strings.Builder

		// dockerfile.WriteString(fmt.Sprintf("FROM %s AS qall-base\n", qallBaseImage))
		dockerfile.WriteString(fmt.Sprintf("FROM %s\n", baseImageName))
		dockerfile.WriteString("WORKDIR /qall-workspace\n")
		dockerfile.WriteString("RUN apt-get update && apt-get install -y curl ca-certificates && rm -rf /var/lib/apt/lists/*\n")
		dockerfile.WriteString("ADD https://astral.sh/uv/install.sh /uv-install.sh\n")
		dockerfile.WriteString("RUN sh /uv-install.sh && rm /uv-install.sh\n")
		dockerfile.WriteString("ENV PATH=\"/root/.local/bin:$PATH\"\n")

		toolInstall := ""

		if qallSource != "" {
			log.Printf(
				"[Docker-Build] Installing qall from local dev dir %s",
				qallSource,
			)
			dockerfile.WriteString("COPY qall /qall-workspace/qall\n")
			toolInstall = "RUN uv tool install /qall-workspace/qall --python 3.13"
		} else {
			log.Printf(
				"[Docker-Build] Installing qall%s from PyPI",
				qallVersionArg(qallVersion),
			)
			toolInstall = fmt.Sprintf(
				"RUN uv tool install qall%s --python 3.13",
				qallVersionArg(qallVersion),
			)
		}

		if len(requirements) > 0 {
			// Strip newlines to prevent basic Dockerfile instruction injection,
			// then fold each requirement into the tool install as a --with dep,
			// so per-task packages land in the same interpreter that later runs
			// `qall task run` (the uv tool venv), not the base image's system
			// python — otherwise the task subprocess cannot import them.

			// Temporary fix to allow the import and use of opencv in an example workflow
			dockerfile.WriteString("RUN apt-get update && apt-get install -y libgl1 libglib2.0-0 && rm -rf /var/lib/apt/lists/*\n")

			for _, reqGroup := range requirements {
				safeRequirements := strings.ReplaceAll(reqGroup, "\n", " ")
				safeRequirements = strings.ReplaceAll(safeRequirements, "\r", "")

				for _, req := range strings.Fields(safeRequirements) {
					toolInstall += fmt.Sprintf(" --with %s", req)
				}
			}
		}

		dockerfile.WriteString("RUN uv python install 3.13\n")
		dockerfile.WriteString(toolInstall + "\n")

		buildCtx, err := createBuildContext(dockerfile.String(), qallSource)
		if err != nil {
			return nil, fmt.Errorf("failed to create build context: %w", err)
		}

		resp, err := d.client.ImageBuild(ctx, buildCtx, client.ImageBuildOptions{
			Tags:    []string{imageTag},
			Remove:  true,
			Version: build.BuilderBuildKit,
		})
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		// Parse the Docker stream to surface actual build errors (e.g., uv install failures)
		if err := jsonmessage.DisplayJSONMessagesStream(resp.Body, io.Discard, 0, false, nil); err != nil {
			return nil, fmt.Errorf("docker build failed: %w", err)
		}

		log.Printf("[Docker-Build] Image %s built successfully.", imageTag)
		return nil, nil
	})

	if err != nil {
		return "", fmt.Errorf("failed to trigger image build: %w", err)
	}

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
	workerSandboxDir := "/tmp/unifs"

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

	// The dataloaders dir is a shared, read-only mount holding one
	// <task_hash>.json per task that references dataloaders. It is provisioned
	// and owned by the client (which resolves the real secrets from the host
	// keyring) — the daemon only forwards it and never parses the credential
	// content. The daemon must NOT create the directory (running as root it
	// would take ownership away from the unprivileged client), so the mount is
	// added only when the client has already provisioned it. Note it is a
	// top-level /dev/shm path, deliberately separate from the root-owned token
	// dir (/dev/shm/qall-secrets).
	if _, err := os.Stat(HostDataloadersDir); err == nil {
		mounts = append(mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   HostDataloadersDir,
			Target:   HostDataloadersDir,
			ReadOnly: true,
		})

		// file:// dataloader sources are host-local directories (e.g.
		// file:///home/me/data). Unlike object-storage sources (s3://, gcs://,
		// ...) — which the task container reaches over the network using the
		// resolved credentials from the secrets file — a host directory is not
		// visible inside a task container unless it is bind-mounted into it.
		// Only the daemon can add mounts when creating the container, so it
		// reads the `src` fields of this task's own secrets file (never the
		// `creds`) and forwards each file:// source read-only at the same path.
		for _, src := range dataloadersFileSources(args.TaskHash) {
			mounts = append(mounts, mount.Mount{
				Type:     mount.TypeBind,
				Source:   src,
				Target:   src,
				ReadOnly: true,
			})
		}
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

	// 	envVariables := []string{
	// 	"PYTHONUNBUFFERED=1",
	// 	fmt.Sprintf("QALL_TOKEN_PATH=%s", workerTokenPath),
	// 	fmt.Sprintf("QALL_DAEMON_ADDRESS=%s:50053", d.daemonContainerID),
	// }

	// if args.EnvironmentVariables != nil {
	// 	for key, value := range args.EnvironmentVariables {
	// 		envVariables = append(envVariables, fmt.Sprintf("%s=%s", key, value))
	// 	}
	// }
	resp, err := d.client.ContainerCreate(ctx, client.ContainerCreateOptions{
		Name: args.Name,
		Config: &container.Config{
			Image: imageTag,
			Env: []string{
				"PYTHONUNBUFFERED=1",
				fmt.Sprintf("QALL_DAEMON_ADDRESS=%s:50053", d.daemonContainerID),
				fmt.Sprintf("QALL_TOKEN_PATH=%s", workerTokenPath),
			},
			Cmd: cmd,
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
	workerSandboxDir := "/tmp/unifs"

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

	resp, err := d.client.ContainerCreate(ctx, client.ContainerCreateOptions{
		Name: containerName,
		Config: &container.Config{
			Image: imageTag,
			Env: []string{
				"PYTHONUNBUFFERED=1",
				fmt.Sprintf("TASK_RUN_ID=%s", args.TaskRunId),
				fmt.Sprintf("QALL_TOKEN_PATH=%s", workerTokenPath),
				fmt.Sprintf("QALL_DAEMON_ADDRESS=%s:50053", d.daemonContainerID),
			},
			Cmd: cmd,
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

// StopAll gracefully stops every running task/worker container spawned by this
// daemon. It is meant to be called during daemon shutdown so that task
// containers are not left running as orphans when the daemon dies.
func (d *DockerRuntime) StopAll(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	containers, err := d.client.ContainerList(ctx, client.ContainerListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	var firstErr error

	for _, c := range containers.Items {
		for _, name := range c.Names {
			if strings.HasPrefix(name, "/qall-task-") || strings.HasPrefix(name, "/qall-worker-") {
				log.Printf("[Daemon-Shutdown] Stopping container %s", name)

				if err := d.Stop(ctx, c.ID); err != nil {
					log.Printf("[Daemon-Shutdown] Failed to stop container %s: %v", name, err)

					if firstErr == nil {
						firstErr = err
					}
				}

				break
			}
		}
	}

	return firstErr
}

func (d *DockerRuntime) Close() error {
	return d.client.Close()
}

// createBuildContext produces the tar stream handed to the Docker build API.
// It always contains the Dockerfile and, when a local qall SDK dev dir is
// configured, also the SDK sources rooted at `qall/` in the context so the
// generated Dockerfile can `COPY qall /qall-workspace/qall`.
func createBuildContext(dockerfile string, devDir string) (io.Reader, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	hdr := &tar.Header{
		Name:    "Dockerfile",
		Mode:    0644,
		Size:    int64(len(dockerfile)),
		ModTime: time.Now(),
	}

	if err := tw.WriteHeader(hdr); err != nil {
		return nil, err
	}

	if _, err := tw.Write([]byte(dockerfile)); err != nil {
		return nil, err
	}

	if devDir != "" {
		if err := addDirToTar(tw, devDir, "qall"); err != nil {
			return nil, err
		}
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}

	return &buf, nil
}

// addDirToTar recursively writes the files of dir into tw, prefixing every
// entry with prefix. Noise (VCS metadata, caches, bytecode) is skipped.
func addDirToTar(tw *tar.Writer, dir string, prefix string) error {
	return filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		if rel == "." {
			return nil
		}

		if entry.IsDir() {
			if shouldSkipDevPath(rel) {
				return filepath.SkipDir
			}
			return nil
		}

		if !entry.Type().IsRegular() || shouldSkipDevPath(rel) {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}

		hdr := &tar.Header{
			Name:    filepath.ToSlash(filepath.Join(prefix, rel)),
			Mode:    int64(info.Mode().Perm()),
			Size:    int64(len(data)),
			ModTime: info.ModTime(),
		}

		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}

		_, err = tw.Write(data)
		return err
	})
}

// shouldSkipDevPath reports whether a path inside the local qall SDK dev dir
// should be excluded from the build context and cache fingerprint (VCS
// metadata, virtualenvs, caches and compiled bytecode).
func shouldSkipDevPath(rel string) bool {
	name := filepath.Base(rel)

	switch name {
	case ".git", ".venv", "__pycache__", ".pytest_cache", ".mypy_cache", ".ruff_cache":
		return true
	}

	return strings.HasSuffix(name, ".pyc")
}

// hashDirContent returns a stable sha256 over the paths and contents of every
// regular file in dir (excluding build-noise directories), so any local change
// to the qall SDK yields a new fingerprint and thus a new runtime image tag.
func hashDirContent(dir string) ([]byte, error) {
	h := sha256.New()

	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		if rel == "." {
			return nil
		}

		if entry.IsDir() {
			if shouldSkipDevPath(rel) {
				return filepath.SkipDir
			}
			return nil
		}

		if !entry.Type().IsRegular() || shouldSkipDevPath(rel) {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		h.Write([]byte(rel))
		h.Write([]byte{0})
		h.Write(data)
		h.Write([]byte{0})
		return nil
	})
	if err != nil {
		return nil, err
	}

	return h.Sum(nil), nil
}

// qallVersionArg returns the version pin suffix for installing `qall`, i.e.
// `==<version>` when a version is provided and an empty string when it is not.
func qallVersionArg(qallVersion string) string {
	if qallVersion == "" {
		return ""
	}

	return fmt.Sprintf("==%s", qallVersion)
}

// dataloadersFileSources returns the host directories backing the `file://`
// dataloader sources referenced by the given task, so the daemon can
// bind-mount them read-only into the task container. It reads only the `src`
// fields of the task's provisioned secrets file
// (`/dev/shm/qall-dataloaders/<task_hash>.json`, mounted into the daemon
// container) and never touches the credential content. Object-storage sources
// and unparseable/missing files yield no mounts.
func dataloadersFileSources(taskHash string) []string {
	data, err := os.ReadFile(filepath.Join(HostDataloadersDir, taskHash+".json"))
	if err != nil {
		return nil
	}
	return dataloadersFileSourcesFrom(data)
}

// dataloadersFileSourcesFrom parses a task's dataloader secrets JSON and
// returns the local host paths for `file://` (or scheme-less absolute) sources.
func dataloadersFileSourcesFrom(data []byte) []string {
	var specs map[string]struct {
		Src string `json:"src"`
	}
	if err := json.Unmarshal(data, &specs); err != nil {
		log.Printf("[Docker-Task] WARNING: could not parse dataloaders secrets file: %v", err)
		return nil
	}

	var sources []string
	seen := make(map[string]bool)
	for _, spec := range specs {
		src := strings.TrimSpace(spec.Src)
		if src == "" {
			continue
		}
		var hostPath string
		switch {
		case strings.HasPrefix(src, "file://"):
			hostPath = strings.TrimPrefix(src, "file://")
		case strings.Contains(src, "://"):
			// Remote object storage: no host mount needed.
			continue
		default:
			hostPath = src
		}
		hostPath = filepath.Clean(hostPath)
		if !filepath.IsAbs(hostPath) || seen[hostPath] {
			continue
		}
		seen[hostPath] = true
		sources = append(sources, hostPath)
	}
	return sources
}
