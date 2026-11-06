package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/plugin"
	"github.com/moby/moby/client"

	"github.com/qall-project/qall-daemon/server/internal/object"
)

// DataloaderPluginDriver is the Docker volume driver used for remote
// dataloaders. It is also the plugin alias the SDK installs
// (rclone/docker-volume-rclone --alias rclone), so the driver name and the
// plugin name resolved by preflight are the same string.
const DataloaderPluginDriver = "rclone"

// DefaultHostVolumesRoot is the host directory bind-mounted 1:1 into the
// daemon (HOST_VOLUMES_ROOT on the SDK side). Dataloader volumes keep their
// rclone VFS cache under
// <root>/dataloaders/<workflow_run_id>/<dataloader_name>/cache.
const DefaultHostVolumesRoot = "/tmp/qall"

// Docker label keys stamped on every daemon-managed dataloader volume so the
// daemon can find them again (run completion, shutdown reaping) without any
// in-memory state.
const (
	// DataloaderLabelDataloader holds the dataloader name.
	DataloaderLabelDataloader = "qall.dataloader"
	// DataloaderLabelWorkflowRunID holds the workflow run the volume belongs to.
	DataloaderLabelWorkflowRunID = "qall.workflow_run_id"
	// DataloaderLabelSrcHash holds a short, credential-free digest of the
	// dataloader source, so a volume created for a different source can be
	// detected without ever storing the source (which may embed credentials).
	DataloaderLabelSrcHash = "qall.dataloader_src_hash"
)

// Sentinel errors returned (wrapped in a *DataloaderMountError) by the
// dataloader volume machinery. The gRPC layer maps them to status codes, so
// they must stay stable and errors.Is-comparable.
var (
	// ErrDataloaderDependencyMissing reports that a host prerequisite (the
	// rclone Docker volume plugin, FUSE, or an architecture-compatible plugin
	// build) is missing.
	ErrDataloaderDependencyMissing = errors.New("dataloader host dependency missing")
	// ErrDataloaderMountFailed reports that the volume itself could not be
	// created, mounted, or removed.
	ErrDataloaderMountFailed = errors.New("dataloader volume mount failed")
	// ErrDataloaderTimeout reports that a Docker volume operation exceeded
	// dataloaderOpTimeout, which usually means a wedged plugin or FUSE mount.
	ErrDataloaderTimeout = errors.New("dataloader volume operation timed out")
)

// dataloaderOpTimeout bounds every Docker volume operation so a wedged plugin
// cannot hang CreateTaskRun (and therefore the whole workflow) forever.
const dataloaderOpTimeout = 30 * time.Second

// DataloaderMountError is the structured error returned for any dataloader
// volume failure. Its message carries the dataloader name, the stage that
// failed and an actionable hint — never credentials.
type DataloaderMountError struct {
	// Dataloader is the dataloader name, or "*" when the failure is not
	// specific to one dataloader (e.g. the plugin preflight).
	Dataloader string
	// Stage is the operation that failed ("preflight", "volume-create",
	// "volume-remove", "mount-target", ...).
	Stage string
	// Err is the underlying error, joined with one of the sentinel errors so
	// errors.Is keeps working for both.
	Err error
	// Hint is the human-actionable remediation (commands, host packages).
	Hint string
}

// Error implements the error interface. It never includes dataloader
// credentials: only the name, the stage, the (already sanitized) underlying
// error and the hint.
func (e *DataloaderMountError) Error() string {
	if e == nil {
		return "<nil>"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "dataloader %q: %s failed", e.Dataloader, e.Stage)
	if e.Err != nil {
		fmt.Fprintf(&b, ": %v", e.Err)
	}
	if e.Hint != "" {
		fmt.Fprintf(&b, " (hint: %s)", e.Hint)
	}

	return b.String()
}

// Unwrap exposes the underlying error so errors.Is/errors.As can traverse it.
func (e *DataloaderMountError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// redactedError keeps the original error chain (Unwrap) while replacing the
// message with a version where secrets have been stripped.
type redactedError struct {
	msg string
	err error
}

func (e *redactedError) Error() string { return e.msg }
func (e *redactedError) Unwrap() error { return e.err }

// redactErrorValues returns err with every occurrence of the given secret
// values replaced by "[redacted]" in its message. The error chain is preserved
// so errors.Is/errors.As keep working. Docker error messages are opaque enough
// that a driver option could theoretically be echoed back; credentials must
// never leave the daemon.
func redactErrorValues(err error, secrets ...string) error {
	if err == nil {
		return nil
	}

	msg := err.Error()
	redacted := false

	for _, secret := range secrets {
		// Short values (e.g. a region or a boolean) are not secrets and
		// replacing them would mangle the message.
		if len(secret) < 8 || !strings.Contains(msg, secret) {
			continue
		}
		msg = strings.ReplaceAll(msg, secret, "[redacted]")
		redacted = true
	}

	if !redacted {
		return err
	}

	return &redactedError{msg: msg, err: err}
}

// normalizeDst mirrors the SDK's normalize_dst (qall/object/dataloader.py): it
// turns a dataloader mount point into a safe *relative* path. "./my_data/",
// "my_data" and "/my_data/" are equivalent and mount at "/my_data" inside the
// task container. Empty targets, the root ("/") and ".." traversal are
// rejected.
func normalizeDst(dst string) (string, error) {
	if strings.TrimSpace(dst) == "" {
		return "", fmt.Errorf("dataloader dst must not be empty")
	}

	norm := strings.ReplaceAll(strings.TrimSpace(dst), `\`, "/")

	for strings.HasPrefix(norm, "./") {
		norm = norm[2:]
	}

	norm = strings.Trim(norm, "/")
	if norm == "" {
		return "", fmt.Errorf("dataloader dst %q must point to a sub-directory, not the root", dst)
	}

	parts := strings.Split(norm, "/")
	for _, part := range parts {
		if part == ".." {
			return "", fmt.Errorf("dataloader dst %q must not contain '..' path traversal", dst)
		}
	}

	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		kept = append(kept, part)
	}

	norm = strings.Join(kept, "/")
	if norm == "" {
		return "", fmt.Errorf("dataloader dst %q normalized to an empty mount point", dst)
	}

	return norm, nil
}

// dataloaderMountTarget returns the absolute path a dataloader is mounted at
// inside the task container: "/" + normalizeDst(dst) ("my_bucket" ->
// "/my_bucket").
func dataloaderMountTarget(dst string) (string, error) {
	norm, err := normalizeDst(dst)
	if err != nil {
		return "", err
	}

	target := "/" + norm
	if target == "/" {
		return "", fmt.Errorf("dataloader dst %q must point to a sub-directory, not the root", dst)
	}

	return target, nil
}

// dataloaderVolumeName returns the deterministic Docker volume name for a
// (workflow_run_id, dataloader_name) pair: "qall-dl-<sha256(runID|name)[:16]>".
// It is stable across daemon restarts and safe as a Docker object name.
func dataloaderVolumeName(workflowRunID, name string) string {
	sum := sha256.Sum256([]byte(workflowRunID + "|" + name))
	return "qall-dl-" + hex.EncodeToString(sum[:])[:16]
}

// dataloaderSrcHash returns a short, credential-free digest of a dataloader
// source, used as a volume label to detect volumes created for a different
// source.
func dataloaderSrcHash(src string) string {
	sum := sha256.Sum256([]byte(src))
	return hex.EncodeToString(sum[:])[:16]
}

// isRemoteDataloaderSrc reports whether src targets a remote object storage
// (s3://, gs://, az://, ...) and therefore needs a kernel mount through the
// rclone plugin. file:// and bare local paths return false: they are plain
// read-only bind mounts.
func isRemoteDataloaderSrc(src string) bool {
	parsed, err := url.Parse(strings.TrimSpace(src))
	if err != nil {
		return false
	}

	scheme := strings.ToLower(parsed.Scheme)

	return scheme != "" && scheme != "file"
}

// dataloaderLocalSources returns the host directories backing the "file://" and
// bare absolute dataloader sources, so the daemon can bind-mount them read-only
// into the task container at the same path. It only reads the Src field and
// never touches credential content. Remote object-storage sources and
// empty/relative entries yield no mounts.
func dataloaderLocalSources(specs []object.DataloaderSpec) []string {
	var sources []string

	seen := make(map[string]bool)

	for _, spec := range specs {
		src := strings.TrimSpace(spec.Src)
		if src == "" || isRemoteDataloaderSrc(src) {
			continue
		}

		hostPath := src
		if strings.HasPrefix(strings.ToLower(src), "file://") {
			hostPath = src[len("file://"):]
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

// rcloneBackendForScheme maps a dataloader URL scheme to the rclone backend
// name. Unknown schemes are rejected so a typo never results in a volume that
// silently reads the wrong remote.
func rcloneBackendForScheme(scheme string) (string, error) {
	switch strings.ToLower(scheme) {
	case "s3", "s3a", "s3n":
		return "s3", nil
	case "gs", "gcs":
		return "gs", nil
	case "az", "azureblob", "adl":
		return "azureblob", nil
	}

	return "", fmt.Errorf(
		"unsupported dataloader backend %q: supported schemes are s3:// (s3, s3a, s3n), gs:// (gs, gcs) and az:// (azureblob, adl)",
		scheme,
	)
}

// normalisedCredentials lower-cases and trims the credential keys so both the
// fsspec spellings (key, secret, endpoint_url) and the cloud-provider ones
// (access_key_id, secret_access_key, ...) resolve to the same mapping. Empty
// values are dropped.
func normalisedCredentials(creds map[string]string) map[string]string {
	out := make(map[string]string, len(creds))

	for key, value := range creds {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out[strings.ToLower(strings.TrimSpace(key))] = value
	}

	return out
}

// lookupCredential returns the first non-empty value among the given
// (already lower-cased) credential keys.
func lookupCredential(creds map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := creds[key]; value != "" {
			return value
		}
	}

	return ""
}

// sanitiseConfigValue strips CR/LF from an rclone config value so a credential
// cannot inject extra INI keys.
func sanitiseConfigValue(value string) string {
	value = strings.ReplaceAll(value, "\r", "")
	return strings.ReplaceAll(value, "\n", "")
}

// buildRcloneINIConfig renders a deterministic, single-section rclone
// configuration. "type" is written first, the remaining keys sorted.
func buildRcloneINIConfig(section string, values map[string]string) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		if key == "type" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var b strings.Builder
	fmt.Fprintf(&b, "[%s]\n", section)
	if backend := values["type"]; backend != "" {
		fmt.Fprintf(&b, "type = %s\n", sanitiseConfigValue(backend))
	}
	for _, key := range keys {
		fmt.Fprintf(&b, "%s = %s\n", key, sanitiseConfigValue(values[key]))
	}

	return b.String()
}

// rcloneConfigForDataloader translates a dataloader spec into the rclone
// "remote" reference and the base64-encoded rclone configuration handed to the
// volume plugin as its "config" driver option.
//
// The configuration has a single section named after the backend (s3, gs,
// azureblob). Each dataloader gets its own volume and therefore its own
// base64 config, so two dataloaders of the same backend never collide even
// with different credentials — and the returned remote stays self-describing
// ("s3:bucket/prefix") and resolvable inside that config.
//
// Credentials are only ever returned inside configBase64; no error produced
// here embeds them.
func rcloneConfigForDataloader(spec object.DataloaderSpec) (remote string, configBase64 string, err error) {
	src := strings.TrimSpace(spec.Src)
	if src == "" {
		return "", "", fmt.Errorf("dataloader %q has an empty source", spec.Name)
	}

	parsed, err := url.Parse(src)
	if err != nil {
		// url.Error embeds the URL, which may carry credentials: never wrap it.
		return "", "", fmt.Errorf("dataloader %q has an unparseable source URL", spec.Name)
	}

	backend, err := rcloneBackendForScheme(parsed.Scheme)
	if err != nil {
		return "", "", fmt.Errorf("dataloader %q: %w", spec.Name, err)
	}

	// rclone expects "<backend>:<bucket or container>/<prefix>", the path
	// being the URL host followed by the URL path, without surrounding
	// slashes.
	objectPath := strings.Trim(parsed.Host+"/"+strings.TrimPrefix(parsed.Path, "/"), "/")
	if objectPath == "" {
		return "", "", fmt.Errorf("dataloader %q source %q does not name a bucket or container", spec.Name, parsed.Scheme+"://")
	}

	remote = fmt.Sprintf("%s:%s", backend, objectPath)

	creds := normalisedCredentials(spec.Credentials)
	values := map[string]string{"type": backend}

	switch backend {
	case "s3":
		// fsspec/s3fs credential names (key, secret, token, endpoint_url,
		// region) plus the AWS SDK spellings.
		if v := lookupCredential(creds, "key", "access_key", "access_key_id", "aws_access_key_id", "access-key-id"); v != "" {
			values["access_key_id"] = v
		}
		if v := lookupCredential(creds, "secret", "secret_key", "secret_access_key", "aws_secret_access_key", "secret-access-key"); v != "" {
			values["secret_access_key"] = v
		}
		if v := lookupCredential(creds, "token", "session_token", "aws_session_token"); v != "" {
			values["session_token"] = v
		}
		if v := lookupCredential(creds, "region", "region_name"); v != "" {
			values["region"] = v
		}
		if endpoint := lookupCredential(creds, "endpoint_url", "endpoint", "client_kwargs_endpoint_url"); endpoint != "" {
			values["endpoint"] = endpoint
			// Self-hosted S3 (MinIO, Ceph, ...) is not an AWS endpoint and
			// usually needs path-style addressing.
			values["provider"] = "Other"
			values["force_path_style"] = "true"
		}
	case "gs":
		// The fsspec GCS "token" is an OAuth access token: it maps to
		// rclone's "token" option, not to service_account_file (which expects
		// a path readable from inside the plugin container).
		if v := lookupCredential(creds, "token", "access_token"); v != "" {
			values["token"] = v
		}
		if v := lookupCredential(creds, "project", "project_id"); v != "" {
			values["project"] = v
		}
		if v := lookupCredential(creds, "key", "client_id"); v != "" {
			values["client_id"] = v
		}
		if v := lookupCredential(creds, "secret", "client_secret"); v != "" {
			values["client_secret"] = v
		}
	case "azureblob":
		if v := lookupCredential(creds, "account_name", "account", "accountName", "azaccount"); v != "" {
			values["account"] = v
		}
		if v := lookupCredential(creds, "account_key", "key", "account-key"); v != "" {
			values["key"] = v
		}
	}

	ini := buildRcloneINIConfig(backend, values)

	return remote, base64.StdEncoding.EncodeToString([]byte(ini)), nil
}

// rcloneDriverOpts returns the complete set of Docker volume driver options
// for a dataloader volume: the rclone remote, its base64 config, a
// write-through VFS cache bounded to 10G, and the per-(run, dataloader) cache
// directory so cached blocks survive across tasks of the same run.
func rcloneDriverOpts(spec object.DataloaderSpec, cacheDir string) (map[string]string, error) {
	remote, configBase64, err := rcloneConfigForDataloader(spec)
	if err != nil {
		return nil, err
	}

	return map[string]string{
		"remote":             remote,
		"config":             configBase64,
		"vfs-cache-mode":     "writes",
		"vfs-cache-max-size": "10G",
		"dir-cache-time":     "1m",
		"poll-interval":      "15s",
		"read-only":          "false",
		"cache-dir":          cacheDir,
		"umask":              "0022",
	}, nil
}

// validateDataloaderName ensures a dataloader name is safe to use as a single
// path component (it becomes part of the host cache directory).
func validateDataloaderName(name string) error {
	if name == "" {
		return fmt.Errorf("dataloader name is required")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("dataloader name %q is not a valid name", name)
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("dataloader name %q must not contain path separators", name)
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("dataloader name %q must not contain control characters", name)
		}
	}

	return nil
}

// dataloaderCacheDir returns the host directory backing the rclone VFS cache of
// one dataloader: <hostVolumesRoot>/dataloaders/<workflow_run_id>/<name>/cache.
// Both path components are validated so the cache can never escape the shared
// host volume root.
func dataloaderCacheDir(hostVolumesRoot, workflowRunID, name string) (string, error) {
	if hostVolumesRoot == "" {
		hostVolumesRoot = DefaultHostVolumesRoot
	}
	if err := validateWorkflowRunID(workflowRunID); err != nil {
		return "", err
	}
	if err := validateDataloaderName(name); err != nil {
		return "", err
	}

	return filepath.Join(hostVolumesRoot, "dataloaders", workflowRunID, name, "cache"), nil
}

// dataloaderRunDir returns the host directory holding every dataloader cache
// directory of a workflow run.
func dataloaderRunDir(hostVolumesRoot, workflowRunID string) (string, error) {
	if hostVolumesRoot == "" {
		hostVolumesRoot = DefaultHostVolumesRoot
	}
	if err := validateWorkflowRunID(workflowRunID); err != nil {
		return "", err
	}

	return filepath.Join(hostVolumesRoot, "dataloaders", workflowRunID), nil
}

// dataloaderPluginHint returns the actionable remediation for a missing or
// disabled rclone plugin. arch is the Docker engine architecture.
func dataloaderPluginHint(arch string) string {
	return fmt.Sprintf(
		"install the rclone Docker volume plugin on the host: docker plugin install rclone/docker-volume-rclone:%s --alias rclone --grant-all-permissions; on Linux also install fuse3 and make sure /dev/fuse is available (qall dataloader setup does this automatically)",
		normalisePluginArch(arch),
	)
}

// normalisePluginArch maps the architecture reported by `docker version` to
// the tag published for the rclone plugin image.
func normalisePluginArch(arch string) string {
	switch strings.ToLower(strings.TrimSpace(arch)) {
	case "x86_64", "x86-64", "amd64":
		return "amd64"
	case "aarch64", "arm64", "arm64v8":
		return "arm64"
	case "armv7l", "armhf", "arm":
		return "arm"
	default:
		if arch == "" {
			return goruntime.GOARCH
		}
		return strings.ToLower(strings.TrimSpace(arch))
	}
}

// isRclonePlugin reports whether the installed plugin is the rclone volume
// plugin, matched either by its install alias (--alias rclone) or by its
// remote reference (rclone/docker-volume-rclone).
func isRclonePlugin(p plugin.Plugin) bool {
	name := strings.ToLower(p.Name)
	if name == DataloaderPluginDriver {
		return true
	}
	if strings.HasPrefix(name, "rclone/docker-volume-rclone") {
		return true
	}
	return strings.Contains(strings.ToLower(p.PluginReference), "rclone/docker-volume-rclone")
}

// newDataloaderMountError builds the structured error for a failed stage,
// classifying the underlying error into one of the sentinel errors so the gRPC
// layer can map it to a status code.
func newDataloaderMountError(dataloader, stage string, err error, hint string) *DataloaderMountError {
	return &DataloaderMountError{
		Dataloader: dataloader,
		Stage:      stage,
		Err:        classifyDataloaderVolumeErr(err),
		Hint:       hint,
	}
}

// classifyDataloaderVolumeErr joins err with the sentinel that describes the
// failure. Joining (instead of replacing) keeps errors.Is working for both the
// sentinel and the original error — notably context.DeadlineExceeded.
func classifyDataloaderVolumeErr(err error) error {
	if err == nil {
		return nil
	}

	// Already classified: keep the error (and its message) as-is.
	if errors.Is(err, ErrDataloaderDependencyMissing) ||
		errors.Is(err, ErrDataloaderTimeout) ||
		errors.Is(err, ErrDataloaderMountFailed) {
		return err
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || errdefs.IsDeadlineExceeded(err) {
		return errors.Join(ErrDataloaderTimeout, err)
	}
	if isDataloaderDependencyError(err) {
		return errors.Join(ErrDataloaderDependencyMissing, err)
	}

	return errors.Join(ErrDataloaderMountFailed, err)
}

// isDataloaderDependencyError reports whether a Docker error indicates a
// missing host dependency (plugin absent/disabled, FUSE unavailable) rather
// than a generic volume failure.
func isDataloaderDependencyError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrDataloaderDependencyMissing) {
		return true
	}

	msg := strings.ToLower(err.Error())
	for _, needle := range []string{"plugin", "no such plugin", "not found", "fuse"} {
		if strings.Contains(msg, needle) {
			return true
		}
	}

	return false
}

// ensureRclonePluginAvailable verifies that the rclone Docker volume plugin is
// installed and enabled before any volume is created, so a missing host
// dependency surfaces as a clean ErrDataloaderDependencyMissing instead of a
// raw Docker error (or a hang).
func (d *DockerRuntime) ensureRclonePluginAvailable(ctx context.Context) error {
	arch := goruntime.GOARCH
	hint := dataloaderPluginHint(arch)

	if d.client == nil {
		return newDataloaderMountError("*", "preflight", ErrDataloaderDependencyMissing, hint)
	}

	opCtx, cancel := context.WithTimeout(ctx, dataloaderOpTimeout)
	defer cancel()

	if version, err := d.client.ServerVersion(opCtx, client.ServerVersionOptions{}); err == nil {
		arch = version.Arch
		hint = dataloaderPluginHint(arch)
	}

	plugins, err := d.client.PluginList(opCtx, client.PluginListOptions{})
	if err != nil {
		return newDataloaderMountError(
			"*",
			"preflight",
			fmt.Errorf("failed to list Docker plugins: %w", err),
			hint,
		)
	}

	for _, p := range plugins.Items {
		if !isRclonePlugin(p) {
			continue
		}
		if !p.Enabled {
			return newDataloaderMountError(
				"*",
				"preflight",
				fmt.Errorf("rclone Docker volume plugin %q is installed but disabled: %w", p.Name, ErrDataloaderDependencyMissing),
				hint+"; enable it with: docker plugin enable "+p.Name,
			)
		}

		return nil
	}

	return newDataloaderMountError(
		"*",
		"preflight",
		fmt.Errorf("rclone Docker volume plugin (alias %q) is not installed: %w", DataloaderPluginDriver, ErrDataloaderDependencyMissing),
		hint,
	)
}

// ensureDataloaderVolume creates (or reuses) the Docker volume backing one
// dataloader of a workflow run. The per-(run, dataloader) host cache directory
// is created first — the plugin needs a pre-existing bind/plugin source — and
// the volume creation is bounded by dataloaderOpTimeout.
func (d *DockerRuntime) ensureDataloaderVolume(ctx context.Context, workflowRunID string, spec object.DataloaderSpec) (string, error) {
	hint := dataloaderPluginHint(goruntime.GOARCH)
	dataloader := spec.Name

	if d.client == nil {
		return "", newDataloaderMountError(dataloader, "volume-create", ErrDataloaderDependencyMissing, hint)
	}

	volName := dataloaderVolumeName(workflowRunID, spec.Name)

	cacheDir, err := dataloaderCacheDir(d.hostVolumesRoot, workflowRunID, spec.Name)
	if err != nil {
		return "", newDataloaderMountError(dataloader, "volume-create", err, "dataloader names and workflow run ids must be safe path components")
	}

	// Bind/plugin volume sources must exist before the plugin mounts them.
	if err := os.MkdirAll(cacheDir, 0o777); err != nil {
		return "", newDataloaderMountError(dataloader, "volume-create", fmt.Errorf("failed to create dataloader cache directory %s: %w", cacheDir, err), hint)
	}

	opts, err := rcloneDriverOpts(spec, cacheDir)
	if err != nil {
		// Never leak credentials: the config option is part of opts.
		return "", newDataloaderMountError(dataloader, "volume-create", redactErrorValues(err, credentialValues(spec)...), "check the dataloader source and credentials")
	}

	labels := map[string]string{
		DataloaderLabelDataloader:    spec.Name,
		DataloaderLabelWorkflowRunID: workflowRunID,
		DataloaderLabelSrcHash:       dataloaderSrcHash(spec.Src),
	}

	create := func() error {
		opCtx, cancel := context.WithTimeout(ctx, dataloaderOpTimeout)
		defer cancel()

		_, err := d.client.VolumeCreate(opCtx, client.VolumeCreateOptions{
			Name:       volName,
			Driver:     DataloaderPluginDriver,
			DriverOpts: opts,
			Labels:     labels,
		})

		return err
	}

	err = create()
	if err != nil && isDataloaderVolumeConflict(err) {
		// A volume with the same name exists with different options (stale
		// credentials, changed dst, ...): remove it and retry exactly once.
		log.Printf("[Dataloader] Volume %s exists with stale options, recreating it", volName)

		if removeErr := d.removeDataloaderVolume(ctx, volName); removeErr != nil {
			return "", newDataloaderMountError(
				dataloader,
				"volume-create",
				redactErrorValues(errors.Join(err, removeErr), opts["config"]),
				hint,
			)
		}

		err = create()
	}

	if err != nil {
		return "", newDataloaderMountError(
			dataloader,
			"volume-create",
			redactErrorValues(err, append(credentialValues(spec), opts["config"])...),
			hint,
		)
	}

	log.Printf("[Dataloader] Volume %s ready for workflow run %s (dataloader %s)", volName, workflowRunID, spec.Name)

	return volName, nil
}

// isDataloaderVolumeConflict reports whether a VolumeCreate error means a
// volume with the same name already exists with different options.
func isDataloaderVolumeConflict(err error) bool {
	if err == nil {
		return false
	}
	if errdefs.IsConflict(err) || errdefs.IsAlreadyExists(err) {
		return true
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "already exists") && (strings.Contains(msg, "option") || strings.Contains(msg, "different"))
}

// credentialValues returns the credential values of a spec, for redaction.
func credentialValues(spec object.DataloaderSpec) []string {
	values := make([]string, 0, len(spec.Credentials))
	for _, value := range spec.Credentials {
		values = append(values, value)
	}

	return values
}

// removeDataloaderVolume removes a dataloader volume. A missing volume is not
// an error: removal is best-effort at the end of a run.
func (d *DockerRuntime) removeDataloaderVolume(ctx context.Context, volName string) error {
	if d.client == nil || volName == "" {
		return nil
	}

	opCtx, cancel := context.WithTimeout(ctx, dataloaderOpTimeout)
	defer cancel()

	if _, err := d.client.VolumeRemove(opCtx, volName, client.VolumeRemoveOptions{}); err != nil {
		if errdefs.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to remove dataloader volume %s: %w", volName, err)
	}

	return nil
}

// removeDataloaderVolumeDir removes the host cache directory of one dataloader
// (best-effort).
func removeDataloaderVolumeDir(hostVolumesRoot, workflowRunID, name string) error {
	cacheDir, err := dataloaderCacheDir(hostVolumesRoot, workflowRunID, name)
	if err != nil {
		return err
	}

	dataloaderDir := filepath.Dir(cacheDir)

	if err := os.RemoveAll(dataloaderDir); err != nil {
		return fmt.Errorf("failed to remove dataloader cache directory %s: %w", dataloaderDir, err)
	}

	return nil
}

// ReapRunDataloaderVolumes removes every dataloader volume (and its host cache
// directory) belonging to a workflow run. It is used on run completion and on
// shutdown and is label-driven, so it also cleans up volumes left behind by a
// previous daemon process.
func (d *DockerRuntime) ReapRunDataloaderVolumes(ctx context.Context, workflowRunID string) error {
	if workflowRunID == "" {
		return nil
	}

	var errs []error

	if d.client != nil {
		opCtx, cancel := context.WithTimeout(ctx, dataloaderOpTimeout)

		filters := make(client.Filters).Add("label", DataloaderLabelWorkflowRunID+"="+workflowRunID)

		volumes, err := d.client.VolumeList(opCtx, client.VolumeListOptions{Filters: filters})
		cancel()

		if err != nil {
			errs = append(errs, fmt.Errorf("failed to list dataloader volumes of workflow run %s: %w", workflowRunID, err))
		} else {
			for _, volume := range volumes.Items {
				if err := d.removeDataloaderVolume(ctx, volume.Name); err != nil {
					errs = append(errs, err)
					continue
				}

				name := volume.Labels[DataloaderLabelDataloader]
				if name == "" {
					continue
				}
				if err := removeDataloaderVolumeDir(d.hostVolumesRoot, workflowRunID, name); err != nil {
					errs = append(errs, err)
				}

				log.Printf("[Dataloader] Reaped volume %s of workflow run %s", volume.Name, workflowRunID)
			}
		}
	}

	// Sweep whatever is left in the run's host directory (empty volumes were
	// already removed above; this drops the now-empty parent directories).
	if runDir, err := dataloaderRunDir(d.hostVolumesRoot, workflowRunID); err == nil {
		if err := os.RemoveAll(runDir); err != nil {
			errs = append(errs, fmt.Errorf("failed to remove dataloader directory %s: %w", runDir, err))
		}
	}

	return errors.Join(errs...)
}

// RemoveDataloaderVolumes removes the named dataloader volumes of a workflow
// run plus their host cache directories. Removal is best-effort: errors are
// aggregated so one stuck volume does not keep the others alive.
func (d *DockerRuntime) RemoveDataloaderVolumes(ctx context.Context, workflowRunID string, names []string) error {
	if len(names) == 0 {
		return nil
	}

	var errs []error

	for _, name := range names {
		volName := dataloaderVolumeName(workflowRunID, name)

		if err := d.removeDataloaderVolume(ctx, volName); err != nil {
			errs = append(errs, err)
		}
		if err := removeDataloaderVolumeDir(d.hostVolumesRoot, workflowRunID, name); err != nil {
			errs = append(errs, err)
		}

		log.Printf("[Dataloader] Removed volume %s of workflow run %s (dataloader %s)", volName, workflowRunID, name)
	}

	return errors.Join(errs...)
}

// dataloaderMounts turns the dataloader specs of a task into container mounts:
// one rclone-backed Docker volume per remote dataloader, mounted read-write at
// "/"+dst, and a read-only bind mount per file:// / bare absolute source.
//
// Volumes created by a failing call are removed again (reverse order) so a
// partially provisioned task never leaks a volume.
func (d *DockerRuntime) dataloaderMounts(ctx context.Context, workflowRunID string, specs []object.DataloaderSpec) ([]mount.Mount, error) {
	if len(specs) == 0 {
		return nil, nil
	}

	hasRemote := false
	for _, spec := range specs {
		if isRemoteDataloaderSrc(spec.Src) {
			hasRemote = true
			break
		}
	}

	// Preflight once per task: a missing plugin must surface even before the
	// first volume is created.
	if hasRemote {
		if err := d.ensureRclonePluginAvailable(ctx); err != nil {
			return nil, err
		}
	}

	var mounts []mount.Mount

	created := make([]string, 0, len(specs))
	seen := make(map[string]bool)

	cleanup := func() {
		for i := len(created) - 1; i >= 0; i-- {
			if err := d.removeDataloaderVolume(ctx, created[i]); err != nil {
				log.Printf("[Dataloader] Failed to roll back volume %s: %v", created[i], err)
			}
		}
	}

	for _, spec := range specs {
		if !isRemoteDataloaderSrc(spec.Src) {
			continue
		}

		target, err := dataloaderMountTarget(spec.Dst)
		if err != nil {
			cleanup()
			return nil, newDataloaderMountError(spec.Name, "mount-target", err, "choose a dst that is a relative sub-directory, e.g. \"my_bucket\" or \"data/mnist\"")
		}

		volName := dataloaderVolumeName(workflowRunID, spec.Name)
		key := volName + "|" + target
		if seen[key] {
			continue
		}
		seen[key] = true

		if _, err := d.ensureDataloaderVolume(ctx, workflowRunID, spec); err != nil {
			cleanup()
			return nil, err
		}

		created = append(created, volName)
		mounts = append(mounts, mount.Mount{
			Type:     mount.TypeVolume,
			Source:   volName,
			Target:   target,
			ReadOnly: false,
		})
	}

	// file:// and bare absolute sources stay plain read-only bind mounts: no
	// FUSE, no credentials and no host dependency.
	for _, src := range dataloaderLocalSources(specs) {
		key := "bind|" + src
		if seen[key] {
			continue
		}
		seen[key] = true

		mounts = append(mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   src,
			Target:   src,
			ReadOnly: true,
		})
	}

	return mounts, nil
}

// volumeMountSources returns the volume names referenced by the given mounts,
// so a failed container creation can roll the just-created volumes back.
func volumeMountSources(mounts []mount.Mount) []string {
	var sources []string

	for _, m := range mounts {
		if m.Type == mount.TypeVolume && m.Source != "" {
			sources = append(sources, m.Source)
		}
	}

	return sources
}

// containerDataloaderError wraps a ContainerCreate/ContainerStart error into a
// DataloaderMountError when it looks dataloader-related (plugin, FUSE, or one
// of the dataloader volumes of this task). It returns nil otherwise so the
// caller keeps its original error.
func containerDataloaderError(stage string, err error, volumeNames []string) error {
	if err == nil {
		return nil
	}

	msg := strings.ToLower(err.Error())

	related := false
	for _, needle := range []string{"plugin", "fuse", "volume"} {
		if strings.Contains(msg, needle) {
			related = true
			break
		}
	}
	if !related {
		for _, name := range volumeNames {
			if name != "" && strings.Contains(err.Error(), name) {
				related = true
				break
			}
		}
	}
	if !related {
		return nil
	}

	return newDataloaderMountError("*", stage, err, dataloaderPluginHint(goruntime.GOARCH))
}
