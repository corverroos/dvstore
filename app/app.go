package app

import (
	"context"
	"github.com/corverroos/dvstore/router"
	"github.com/corverroos/dvstore/service"
	"github.com/obolnetwork/charon/app/errors"
	"github.com/obolnetwork/charon/app/log"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"net/http"
	"time"
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

	defSvc := service.NewDefinition(client.Database("dvstore").Collection("definitions"))

	mux, err := router.NewRouter(defSvc)
	if err != nil {
		return errors.Wrap(err, "failed to create router")
	}

	server := http.Server{Addr: conf.HTTPAddress, Handler: mux, ReadHeaderTimeout: time.Second}
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		log.Info(ctx, "Shutdown detected")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second) // Fresh shutdown context.
		defer cancel()
		err = server.Shutdown(shutdownCtx)
		if err != nil {
			return errors.Wrap(err, "failed to shutdown server")
		}
	case err := <-serverErr:
		return errors.Wrap(err, "server error")
	}

	log.Info(ctx, "Good bye 👋")

	return nil
}
