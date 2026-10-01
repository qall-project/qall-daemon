package registry

import (
	"context"
	"fmt"

	"github.com/qall-project/qall-registry/server/pkg/core"
)

type LocalRegistry struct {
	core *core.RegistryCore
}

func NewLocalRegistry(core *core.RegistryCore) *LocalRegistry {
	return &LocalRegistry{core: core}
}

func (l *LocalRegistry) Resolve(ctx context.Context, name, version string) (string, error) {
	rootHash, _, err := l.core.Resolve(ctx, name, version)
	return rootHash, err
}

func (l *LocalRegistry) GetBlock(ctx context.Context, hash string) ([]byte, error) {
	blocks, err := l.core.GetBlocks(ctx, []string{hash})
	if err != nil || len(blocks) == 0 {
		return nil, fmt.Errorf("bloc local introuvable: %v", err)
	}
	return blocks[0].Data, nil
}
