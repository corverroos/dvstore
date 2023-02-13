package app

import (
	"context"
	"github.com/obolnetwork/charon/app/errors"
	"github.com/obolnetwork/charon/app/log"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Config struct {
	Log         log.Config
	HTTPAddress string
	MongoURL    string
}

func Run(ctx context.Context, conf Config) (err error) {
	defer func() {
		if err != nil {
			log.Error(ctx, "Fatal error", err)
		}
	}()
	ctx = log.WithTopic(ctx, "app")

	log.Info(ctx, "Starting dvstore")

	client, err := mongo.NewClient(options.Client().ApplyURI(conf.MongoURL))
	if err != nil {
		return errors.Wrap(err, "failed to create mongo client")
	}
	err = client.Connect(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to connect to mongo")
	}
	defer client.Disconnect(ctx)

	return nil
}
