package registry

import (
	"context"
	"fmt"

	pbRegistry "qall-daemon-server/internal/registry/protobuf/registry_api_v1"
)

type RemoteRegistry struct {
	client pbRegistry.ApiClient
}

func NewRemoteRegistry(client pbRegistry.ApiClient) *RemoteRegistry {
	return &RemoteRegistry{client: client}
}

func (r *RemoteRegistry) Resolve(ctx context.Context, name, version string) (string, error) {
	resp, err := r.client.Resolve(ctx, &pbRegistry.ResolveRequest{Name: name, Version: version})
	if err != nil {
		return "", err
	}
	return resp.RootHash, nil
}

func (r *RemoteRegistry) GetBlock(ctx context.Context, hash string) ([]byte, error) {
	resp, err := r.client.Get(ctx, &pbRegistry.GetRequest{Hashes: []string{hash}})

	if err != nil || len(resp.Blocks) == 0 {
		return nil, fmt.Errorf("bloc introuvable ou erreur réseau: %v", err)
	}

	return resp.Blocks[0].Data, nil
}
