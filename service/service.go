package service

import (
	"context"
	"github.com/obolnetwork/charon/cluster"
)

type Definition interface {
	Get(ctx context.Context, configHash []byte) (cluster.Definition, error)
	Delete(ctx context.Context, configHash []byte) error
	Create(ctx context.Context, def cluster.Definition) error
	AddOperator(ctx context.Context, configHash []byte, forkVersion []byte, operator cluster.Operator) error
}
