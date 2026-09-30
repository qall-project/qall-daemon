package server

import "context"

type APIServer interface {
	Start(ctx context.Context) error
}
