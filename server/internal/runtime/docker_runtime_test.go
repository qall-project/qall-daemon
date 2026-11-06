package runtime

import (
	"net/netip"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/moby/moby/api/types/network"

	"github.com/qall-project/qall-daemon/server/internal/object"
)

func TestBridgeGatewayIP(t *testing.T) {
	tests := []struct {
		name    string
		netw    *network.Inspect
		want    string
		wantErr bool
	}{
		{
			name: "bridge network with gateway",
			netw: &network.Inspect{
				Network: network.Network{
					IPAM: network.IPAM{
						Driver: "default",
						Config: []network.IPAMConfig{
							{
								Subnet:  netip.MustParsePrefix("172.18.0.0/16"),
								Gateway: netip.MustParseAddr("172.18.0.1"),
							},
						},
					},
				},
			},
			want: "172.18.0.1",
		},
		{
			name: "no gateway configured",
			netw: &network.Inspect{
				Network: network.Network{
					IPAM: network.IPAM{Config: []network.IPAMConfig{}},
				},
			},
			wantErr: true,
		},
		{
			name:    "empty ipam config",
			netw:    &network.Inspect{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := bridgeGatewayIP(tt.netw)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got gateway %q", got)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tt.want {
				t.Fatalf("got gateway %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMappingSandboxDir(t *testing.T) {
	tests := []struct {
		name    string
		runID   string
		want    string
		wantErr bool
	}{
		{
			name:  "valid workflow run id",
			runID: "4f0b1f3a-7c2e-4b6d-9f8a-1a2b3c4d5e6f",
			want:  filepath.Join(HostSandboxDir, "4f0b1f3a-7c2e-4b6d-9f8a-1a2b3c4d5e6f"),
		},
		{
			name:    "empty run id is rejected",
			runID:   "",
			wantErr: true,
		},
		{
			name:    "path traversal is rejected",
			runID:   "..",
			wantErr: true,
		},
		{
			name:    "absolute path is rejected",
			runID:   "/etc",
			wantErr: true,
		},
		{
			name:    "nested separator is rejected",
			runID:   "run/aa",
			wantErr: true,
		},
		{
			name:    "backslash is rejected",
			runID:   `run\aa`,
			wantErr: true,
		},
		{
			name:    "control character is rejected",
			runID:   "run\x00aa",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mappingSandboxDir(tt.runID)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error for run id %q, got dir %q", tt.runID, got)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tt.want {
				t.Fatalf("got dir %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDataloaderLocalSources(t *testing.T) {
	tests := []struct {
		name  string
		specs []object.DataloaderSpec
		want  []string
	}{
		{
			name:  "no specs",
			specs: nil,
			want:  nil,
		},
		{
			name: "file scheme source is stripped",
			specs: []object.DataloaderSpec{
				{Src: "file:///home/me/data"},
			},
			want: []string{"/home/me/data"},
		},
		{
			name: "bare absolute source is kept",
			specs: []object.DataloaderSpec{
				{Src: "/home/me/data"},
			},
			want: []string{"/home/me/data"},
		},
		{
			name: "object storage sources are skipped",
			specs: []object.DataloaderSpec{
				{Src: "s3://bucket/path"},
				{Src: "gs://bucket/path"},
			},
			want: nil,
		},
		{
			name: "relative and empty sources are skipped",
			specs: []object.DataloaderSpec{
				{Src: ""},
				{Src: "  "},
				{Src: "relative/path"},
			},
			want: nil,
		},
		{
			name: "duplicate sources are deduplicated",
			specs: []object.DataloaderSpec{
				{Src: "file:///home/me/data"},
				{Src: "/home/me/data"},
			},
			want: []string{"/home/me/data"},
		},
		{
			name: "mixed sources",
			specs: []object.DataloaderSpec{
				{Src: "file:///home/me/data"},
				{Src: "s3://bucket/path"},
				{Src: "/opt/shared"},
				{Src: ""},
			},
			want: []string{"/home/me/data", "/opt/shared"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dataloaderLocalSources(tt.specs)

			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
