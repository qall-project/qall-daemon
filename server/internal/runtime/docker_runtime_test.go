package runtime

import (
	"net/netip"
	"testing"

	"github.com/moby/moby/api/types/network"
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
