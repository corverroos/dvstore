package service

import (
	"context"
	"github.com/obolnetwork/charon/app/errors"
	"github.com/obolnetwork/charon/cluster"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type Definition interface {
	Get(ctx context.Context, configHash []byte) (cluster.Definition, error)
	Delete(ctx context.Context, configHash []byte) error
	Create(ctx context.Context, def cluster.Definition) error
	AddOperator(ctx context.Context, configHash []byte, forkVersion []byte, operator cluster.Operator) error
}

func NewDefinition(table *mongo.Collection) Definition {
	return &definitionImpl{
		table: table,
	}
}

type definitionImpl struct {
	table *mongo.Collection
}

func (d definitionImpl) Get(ctx context.Context, configHash []byte) (cluster.Definition, error) {
	res := d.table.FindOne(ctx, bson.D{{"config_hash", configHash}})
	if errors.Is(res.Err(), mongo.ErrNoDocuments) {
		return cluster.Definition{}, errors.Wrap(ErrNotFound, "definition not found")
	} else if res.Err() != nil {
		return cluster.Definition{}, errors.Wrap(res.Err(), "failed to get definition")
	}

	var def cluster.Definition
	err := res.Decode(&def)
	if err != nil {
		return cluster.Definition{}, errors.Wrap(err, "failed to decode definition")
	}

	return def, nil
}

func (d definitionImpl) Delete(ctx context.Context, configHash []byte) error {
	res, err := d.table.DeleteOne(ctx, bson.D{{"config_hash", configHash}})
	if err != nil {
		return errors.Wrap(err, "failed to delete definition")
	} else if res.DeletedCount == 0 {
		return errors.Wrap(ErrNotFound, "definition not found")
	}

	return nil
}

func (d definitionImpl) Create(ctx context.Context, def cluster.Definition) error {
	_, err := d.table.InsertOne(ctx, def)
	if err != nil {
		return errors.Wrap(err, "failed to create definition")
	}

	return nil
}

func (d definitionImpl) AddOperator(ctx context.Context, configHash []byte, forkVersion []byte, operator cluster.Operator) error {
	res := d.table.FindOneAndUpdate(ctx,
		bson.D{{"config_hash", configHash}},
		bson.D{{"$addToSet", bson.D{{"operators", operator}}}},
	)
	if errors.Is(res.Err(), mongo.ErrNoDocuments) {
		return errors.Wrap(ErrNotFound, "definition not found")
	} else if res.Err() != nil {
		return errors.Wrap(res.Err(), "failed to get definition")
	}

	return nil
}
