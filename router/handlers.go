package router

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"github.com/corverroos/dvstore/service"
	"github.com/obolnetwork/charon/cluster"
	"net/http"
	"net/url"
	"strings"
)

func getDefinition(svc service.Definition) handlerFunc {
	return func(ctx context.Context, params map[string]string, query url.Values, body []byte) (res interface{}, err error) {
		hash, ok, err := hexQuery(query, "config_hash")
		if err != nil {
			return nil, err
		} else if !ok {
			return nil, apiError{
				StatusCode: http.StatusBadRequest,
				Message:    "Missing config_hash",
			}
		}

		return svc.Get(ctx, hash)
	}
}

func deleteDefinition(svc service.Definition) handlerFunc {
	return func(ctx context.Context, params map[string]string, query url.Values, body []byte) (res interface{}, err error) {
		hash, ok, err := hexQuery(query, "config_hash")
		if err != nil {
			return nil, err
		} else if !ok {
			return nil, apiError{
				StatusCode: http.StatusBadRequest,
				Message:    "Missing config_hash",
			}
		}

		return nil, svc.Delete(ctx, hash)
	}
}

func createDefinition(svc service.Definition) handlerFunc {
	return func(ctx context.Context, params map[string]string, query url.Values, body []byte) (res interface{}, err error) {
		var def cluster.Definition
		if err := json.Unmarshal(body, &def); err != nil {
			return nil, apiError{
				StatusCode: http.StatusBadRequest,
				Message:    "Invalid body",
				Err:        err,
			}
		}

		if err := def.VerifyHashes(); err != nil {
			return nil, apiError{
				StatusCode: http.StatusBadRequest,
				Message:    "Invalid definition hash",
				Err:        err,
			}
		}

		if err := def.VerifySignatures(); err != nil {
			return nil, apiError{
				StatusCode: http.StatusBadRequest,
				Message:    "Invalid definition signature",
				Err:        err,
			}
		}

		return nil, svc.Create(ctx, def)
	}
}

func addOperator(svc service.Definition) handlerFunc {
	return func(ctx context.Context, params map[string]string, query url.Values, body []byte) (res interface{}, err error) {
		hash, ok, err := hexQuery(query, "config_hash")
		if err != nil {
			return nil, err
		} else if !ok {
			return nil, apiError{
				StatusCode: http.StatusBadRequest,
				Message:    "Missing config_hash",
			}
		}

		req := struct {
			cluster.Operator
			ForkVersion string
		}{}
		if err := json.Unmarshal(body, &req); err != nil {
			return nil, apiError{
				StatusCode: http.StatusBadRequest,
				Message:    "Invalid body",
				Err:        err,
			}
		}

		forkVersion, err := hex.DecodeString(strings.TrimPrefix(req.ForkVersion, "0x"))
		if err != nil {
			return nil, apiError{
				StatusCode: http.StatusBadRequest,
				Message:    "Invalid fork version hex",
				Err:        err,
			}
		}

		return nil, svc.AddOperator(ctx, hash, forkVersion, req.Operator)
	}
}
