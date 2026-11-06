package runtime

import (
	"context"
	"encoding/base64"
	"errors"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/plugin"

	"github.com/qall-project/qall-daemon/server/internal/object"
)

func TestNormalizeDst(t *testing.T) {
	tests := []struct {
		name    string
		dst     string
		want    string
		wantErr bool
	}{
		{name: "empty is rejected", dst: "", wantErr: true},
		{name: "blank is rejected", dst: "  ", wantErr: true},
		{name: "root is rejected", dst: "/", wantErr: true},
		{name: "traversal is rejected", dst: "..", wantErr: true},
		{name: "inner traversal is rejected", dst: "a/../b", wantErr: true},
		{name: "leading ./ and trailing / are stripped", dst: "./my_data/", want: "my_data"},
		{name: "plain relative path is kept", dst: "my_data", want: "my_data"},
		{name: "absolute path is made relative", dst: "/my_data/", want: "my_data"},
		{name: "backslashes become separators", dst: `a\b`, want: "a/b"},
		{name: "nested path is kept", dst: "data/mnist", want: "data/mnist"},
		{name: "nested absolute path is kept", dst: "/data/mnist/", want: "data/mnist"},
		{name: "surrounding whitespace is trimmed", dst: "  my_bucket  ", want: "my_bucket"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeDst(tt.dst)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error for dst %q, got %q", tt.dst, got)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tt.want {
				t.Fatalf("normalizeDst(%q) = %q, want %q", tt.dst, got, tt.want)
			}
		})
	}
}

func TestDataloaderMountTarget(t *testing.T) {
	tests := []struct {
		name    string
		dst     string
		want    string
		wantErr bool
	}{
		{name: "plain name", dst: "my_bucket", want: "/my_bucket"},
		{name: "absolute name", dst: "/my_bucket/", want: "/my_bucket"},
		{name: "nested name", dst: "data/mnist", want: "/data/mnist"},
		{name: "root is rejected", dst: "/", wantErr: true},
		{name: "traversal is rejected", dst: "..", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := dataloaderMountTarget(tt.dst)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error for dst %q, got %q", tt.dst, got)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tt.want {
				t.Fatalf("dataloaderMountTarget(%q) = %q, want %q", tt.dst, got, tt.want)
			}
		})
	}
}

func TestDataloaderVolumeName(t *testing.T) {
	pattern := regexp.MustCompile(`^qall-dl-[0-9a-f]{16}$`)

	first := dataloaderVolumeName("run-1", "mnist")

	if !pattern.MatchString(first) {
		t.Fatalf("volume name %q does not match %q", first, pattern)
	}

	if len(first) > 63 {
		t.Fatalf("volume name %q is longer than the Docker limit (63)", first)
	}

	if second := dataloaderVolumeName("run-1", "mnist"); second != first {
		t.Fatalf("volume name is not deterministic: %q != %q", second, first)
	}

	if other := dataloaderVolumeName("run-2", "mnist"); other == first {
		t.Fatalf("different workflow runs must yield different volume names (%q)", other)
	}

	if other := dataloaderVolumeName("run-1", "coco"); other == first {
		t.Fatalf("different dataloaders must yield different volume names (%q)", other)
	}
}

func TestIsRemoteDataloaderSrc(t *testing.T) {
	tests := []struct {
		src  string
		want bool
	}{
		{src: "s3://bucket/path", want: true},
		{src: "s3a://bucket/path", want: true},
		{src: "gs://bucket/path", want: true},
		{src: "gcs://bucket/path", want: true},
		{src: "az://container/path", want: true},
		{src: "azureblob://container/path", want: true},
		{src: "file:///home/me/data", want: false},
		{src: "/home/me/data", want: false},
		{src: "relative/path", want: false},
		{src: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			if got := isRemoteDataloaderSrc(tt.src); got != tt.want {
				t.Fatalf("isRemoteDataloaderSrc(%q) = %v, want %v", tt.src, got, tt.want)
			}
		})
	}
}

func TestRcloneConfigForDataloaderS3(t *testing.T) {
	spec := object.DataloaderSpec{
		Name: "mnist",
		Src:  "s3://bucket/prefix",
		Dst:  "mnist",
		Credentials: map[string]string{
			"key":          "AKIAEXAMPLEKEY",
			"secret":       "supersecretvalue",
			"endpoint_url": "http://minio:9000",
			"region":       "fr-par",
			"token":        "sessiontokenvalue",
		},
	}

	remote, configBase64, err := rcloneConfigForDataloader(spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if remote != "s3:bucket/prefix" {
		t.Fatalf("remote = %q, want %q", remote, "s3:bucket/prefix")
	}

	decoded, err := base64.StdEncoding.DecodeString(configBase64)
	if err != nil {
		t.Fatalf("config is not valid base64: %v", err)
	}

	ini := string(decoded)
	for _, want := range []string{
		"[s3]",
		"type = s3",
		"access_key_id = AKIAEXAMPLEKEY",
		"secret_access_key = supersecretvalue",
		"endpoint = http://minio:9000",
		"session_token = sessiontokenvalue",
		"region = fr-par",
		"provider = Other",
		"force_path_style = true",
	} {
		if !strings.Contains(ini, want) {
			t.Fatalf("config %q does not contain %q", ini, want)
		}
	}
}

func TestRcloneConfigForDataloaderS3WithoutEndpoint(t *testing.T) {
	spec := object.DataloaderSpec{
		Name:        "aws",
		Src:         "s3://bucket",
		Credentials: map[string]string{"access_key_id": "AKIAEXAMPLEKEY", "secret_access_key": "supersecretvalue"},
	}

	remote, configBase64, err := rcloneConfigForDataloader(spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if remote != "s3:bucket" {
		t.Fatalf("remote = %q, want %q", remote, "s3:bucket")
	}

	decoded, err := base64.StdEncoding.DecodeString(configBase64)
	if err != nil {
		t.Fatalf("config is not valid base64: %v", err)
	}

	ini := string(decoded)
	if strings.Contains(ini, "provider = Other") {
		t.Fatalf("provider = Other must only be set when an endpoint is configured: %q", ini)
	}
	if strings.Contains(ini, "force_path_style") {
		t.Fatalf("force_path_style must only be set when an endpoint is configured: %q", ini)
	}
}

func TestRcloneConfigForDataloaderAzure(t *testing.T) {
	spec := object.DataloaderSpec{
		Name: "blobs",
		Src:  "az://container/data",
		Credentials: map[string]string{
			"account_name": "myaccount",
			"account_key":  "supersecretaccountkey",
		},
	}

	remote, configBase64, err := rcloneConfigForDataloader(spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if remote != "azureblob:container/data" {
		t.Fatalf("remote = %q, want %q", remote, "azureblob:container/data")
	}

	decoded, err := base64.StdEncoding.DecodeString(configBase64)
	if err != nil {
		t.Fatalf("config is not valid base64: %v", err)
	}

	ini := string(decoded)
	for _, want := range []string{"type = azureblob", "account = myaccount", "key = supersecretaccountkey"} {
		if !strings.Contains(ini, want) {
			t.Fatalf("config %q does not contain %q", ini, want)
		}
	}
}

func TestRcloneConfigForDataloaderUnsupportedScheme(t *testing.T) {
	spec := object.DataloaderSpec{
		Name:        "ftp",
		Src:         "ftp://bucket/prefix",
		Credentials: map[string]string{"secret": "supersecretvalue"},
	}

	_, _, err := rcloneConfigForDataloader(spec)
	if err == nil {
		t.Fatal("expected an error for an unsupported scheme")
	}

	if !strings.Contains(err.Error(), "unsupported dataloader backend") {
		t.Fatalf("unexpected error message: %v", err)
	}

	if strings.Contains(err.Error(), "supersecretvalue") {
		t.Fatalf("error message leaks credentials: %v", err)
	}
}

func TestRcloneDriverOpts(t *testing.T) {
	spec := object.DataloaderSpec{
		Name:        "mnist",
		Src:         "s3://bucket/prefix",
		Credentials: map[string]string{"key": "AKIAEXAMPLEKEY", "secret": "supersecretvalue"},
	}

	opts, err := rcloneDriverOpts(spec, "/tmp/qall/dataloaders/run/mnist/cache")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := map[string]string{
		"remote":             "s3:bucket/prefix",
		"vfs-cache-mode":     "writes",
		"vfs-cache-max-size": "10G",
		"dir-cache-time":     "1m",
		"poll-interval":      "15s",
		"read-only":          "false",
		"cache-dir":          "/tmp/qall/dataloaders/run/mnist/cache",
		"umask":              "0022",
	}

	for key, value := range want {
		if got := opts[key]; got != value {
			t.Fatalf("opts[%q] = %q, want %q", key, got, value)
		}
	}

	configBase64, ok := opts["config"]
	if !ok || configBase64 == "" {
		t.Fatalf("missing config driver option: %+v", opts)
	}

	if _, err := base64.StdEncoding.DecodeString(configBase64); err != nil {
		t.Fatalf("config driver option is not valid base64: %v", err)
	}
}

func TestRcloneDriverOptsErrorDoesNotLeakCredentials(t *testing.T) {
	spec := object.DataloaderSpec{
		Name:        "nope",
		Src:         "ftp://bucket/prefix",
		Credentials: map[string]string{"secret": "supersecretvalue"},
	}

	_, err := rcloneDriverOpts(spec, "/tmp/cache")
	if err == nil {
		t.Fatal("expected an error for an unsupported scheme")
	}

	if strings.Contains(err.Error(), "supersecretvalue") {
		t.Fatalf("error message leaks credentials: %v", err)
	}
}

func TestRedactErrorValues(t *testing.T) {
	original := errors.New("docker said: supersecretvalue is wrong")
	redacted := redactErrorValues(original, "supersecretvalue")

	if strings.Contains(redacted.Error(), "supersecretvalue") {
		t.Fatalf("secret was not redacted: %v", redacted)
	}

	if !errors.Is(redacted, original) {
		t.Fatal("redaction broke the error chain")
	}
}

func TestDataloaderMountErrorTyping(t *testing.T) {
	// A plugin-missing Docker error must classify as a missing dependency.
	cause := errors.New("Error response from daemon: plugin \"rclone\" not found")
	err := newDataloaderMountError("*", "preflight", cause, dataloaderPluginHint("amd64"))

	if !errors.Is(err, ErrDataloaderDependencyMissing) {
		t.Fatalf("expected ErrDataloaderDependencyMissing, got %v", err)
	}

	var mountErr *DataloaderMountError
	if !errors.As(err, &mountErr) {
		t.Fatalf("expected a *DataloaderMountError, got %T", err)
	}

	message := err.Error()
	for _, want := range []string{"*", "preflight", "hint", "docker plugin install rclone/docker-volume-rclone:amd64"} {
		if !strings.Contains(message, want) {
			t.Fatalf("error message %q does not contain %q", message, want)
		}
	}

	// Timeouts must be both a dataloader timeout and a context deadline, so
	// callers can branch on either.
	timeoutErr := newDataloaderMountError("mnist", "volume-create", context.DeadlineExceeded, "retry later")
	if !errors.Is(timeoutErr, ErrDataloaderTimeout) {
		t.Fatalf("expected ErrDataloaderTimeout, got %v", timeoutErr)
	}
	if !errors.Is(timeoutErr, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded in the chain, got %v", timeoutErr)
	}

	// Generic failures must map to ErrDataloaderMountFailed.
	genericErr := newDataloaderMountError("mnist", "volume-create", errors.New("boom"), "check the daemon logs")
	if !errors.Is(genericErr, ErrDataloaderMountFailed) {
		t.Fatalf("expected ErrDataloaderMountFailed, got %v", genericErr)
	}
}

func TestNormalisePluginArch(t *testing.T) {
	tests := map[string]string{
		"x86_64":  "amd64",
		"amd64":   "amd64",
		"aarch64": "arm64",
		"arm64":   "arm64",
		"armv7l":  "arm",
	}

	for arch, want := range tests {
		if got := normalisePluginArch(arch); got != want {
			t.Fatalf("normalisePluginArch(%q) = %q, want %q", arch, got, want)
		}
	}
}

func TestIsRclonePlugin(t *testing.T) {
	tests := []struct {
		name   string
		plugin plugin.Plugin
		want   bool
	}{
		{name: "install alias", plugin: plugin.Plugin{Name: "rclone"}, want: true},
		{name: "remote reference", plugin: plugin.Plugin{Name: "rclone/docker-volume-rclone:amd64"}, want: true},
		{name: "remote reference only", plugin: plugin.Plugin{Name: "some-alias", PluginReference: "rclone/docker-volume-rclone:amd64"}, want: true},
		{name: "unrelated plugin", plugin: plugin.Plugin{Name: "local-persist"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRclonePlugin(tt.plugin); got != tt.want {
				t.Fatalf("isRclonePlugin(%+v) = %v, want %v", tt.plugin, got, tt.want)
			}
		})
	}
}

func TestDataloaderCacheDir(t *testing.T) {
	got, err := dataloaderCacheDir("/tmp/qall", "run-1", "mnist")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := filepath.Join("/tmp/qall", "dataloaders", "run-1", "mnist", "cache")
	if got != want {
		t.Fatalf("dataloaderCacheDir = %q, want %q", got, want)
	}

	if got, err := dataloaderCacheDir("", "run-1", "mnist"); err != nil || !strings.HasPrefix(got, DefaultHostVolumesRoot) {
		t.Fatalf("empty root must default to %q, got %q (err: %v)", DefaultHostVolumesRoot, got, err)
	}

	invalid := []struct {
		name   string
		runID  string
		loader string
	}{
		{name: "empty run id", runID: "", loader: "mnist"},
		{name: "traversing run id", runID: "..", loader: "mnist"},
		{name: "separator in run id", runID: "run/1", loader: "mnist"},
		{name: "empty dataloader name", runID: "run-1", loader: ""},
		{name: "dot dataloader name", runID: "run-1", loader: "."},
		{name: "traversing dataloader name", runID: "run-1", loader: ".."},
		{name: "separator in dataloader name", runID: "run-1", loader: "a/b"},
		{name: "control character in dataloader name", runID: "run-1", loader: "a\x00b"},
	}

	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := dataloaderCacheDir("/tmp/qall", tt.runID, tt.loader); err == nil {
				t.Fatalf("expected an error, got %q", got)
			}
		})
	}
}

func TestDataloaderMountsLocalOnly(t *testing.T) {
	daemonRuntime := &DockerRuntime{hostVolumesRoot: t.TempDir()}

	mounts, err := daemonRuntime.dataloaderMounts(context.Background(), "run-1", []object.DataloaderSpec{
		{Name: "local", Src: "file:///tmp/qall-test-data", Dst: "data"},
		{Name: "local-bis", Src: "/tmp/qall-test-data", Dst: "data-bis"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mounts) != 1 {
		t.Fatalf("expected one deduplicated bind mount, got %+v", mounts)
	}

	got := mounts[0]
	if got.Type != mount.TypeBind || got.Source != "/tmp/qall-test-data" || got.Target != "/tmp/qall-test-data" || !got.ReadOnly {
		t.Fatalf("unexpected local mount: %+v", got)
	}
}

func TestDataloaderMountsRemoteWithoutPluginClient(t *testing.T) {
	daemonRuntime := &DockerRuntime{hostVolumesRoot: t.TempDir()}

	_, err := daemonRuntime.dataloaderMounts(context.Background(), "run-1", []object.DataloaderSpec{
		{Name: "mnist", Src: "s3://bucket/prefix", Dst: "mnist"},
	})
	if err == nil {
		t.Fatal("expected an error without a Docker client")
	}

	if !errors.Is(err, ErrDataloaderDependencyMissing) {
		t.Fatalf("expected ErrDataloaderDependencyMissing, got %v", err)
	}
}

func TestVolumeMountSources(t *testing.T) {
	mounts := []mount.Mount{
		{Type: mount.TypeBind, Source: "/tmp/data"},
		{Type: mount.TypeVolume, Source: "qall-dl-1"},
		{Type: mount.TypeVolume, Source: "qall-dl-2"},
	}

	got := volumeMountSources(mounts)

	if len(got) != 2 || got[0] != "qall-dl-1" || got[1] != "qall-dl-2" {
		t.Fatalf("volumeMountSources = %v", got)
	}
}
