package app

import (
	"context"
	"github.com/obolnetwork/charon/app/log"
)

type Config struct {
	Log         log.Config
	HTTPAddress string
}

func Run(ctx context.Context, conf Config) (err error) {
	defer func() {
		if err != nil {
			log.Error(ctx, "Fatal error", err)
		}
	}()
	ctx = log.WithTopic(ctx, "app")

	log.Info(ctx, "Starting dvstore")

	return nil
}
