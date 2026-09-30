package registry

import "context"

type RegistryProvider interface {
	Resolve(ctx context.Context, name, version string) (string, error)
	GetBlock(ctx context.Context, hash string) ([]byte, error)
}
