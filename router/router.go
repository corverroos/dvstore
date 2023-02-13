package router

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/corverroos/dvstore/service"
	"github.com/gorilla/mux"
	"github.com/obolnetwork/charon/app/errors"
	"github.com/obolnetwork/charon/app/log"
	"github.com/obolnetwork/charon/app/z"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func NewRouter(defSvc service.Definition) (*mux.Router, error) {
	endpoints := []struct {
		Name    string
		Path    string
		Method  string
		Handler handlerFunc
	}{
		{
			Name:    "get_definition",
			Method:  http.MethodGet,
			Path:    "/dv/{config_hash}",
			Handler: getDefinition(defSvc),
		},
		{
			Name:    "delete_definition",
			Method:  http.MethodDelete,
			Path:    "/dv/{config_hash}",
			Handler: deleteDefinition(defSvc),
		},
		{
			Name:    "create_definition",
			Method:  http.MethodPost,
			Path:    "/dv",
			Handler: createDefinition(defSvc),
		},
		{
			Name:    "add_operator",
			Method:  http.MethodPut,
			Path:    "/dv/{config_hash}",
			Handler: addOperator(defSvc),
		},
	}

	r := mux.NewRouter()
	for _, e := range endpoints {
		r.Handle(e.Path, wrap(e.Name, e.Handler))
	}

	return r, nil
}

// apiErr defines a validator api error that is converted to an eth2 errorResponse.
type apiError struct {
	// StatusCode is the http status code to return, defaults to 500.
	StatusCode int
	// Message is a safe human-readable message, defaults to "Internal server error".
	Message string
	// Err is the original error, returned in debug mode.
	Err error
}

func (a apiError) Error() string {
	return fmt.Sprintf("api error[status=%d,msg=%s]: %v", a.StatusCode, a.Message, a.Err)
}

// handlerFunc is a convenient handler function providing a context, parsed path parameters,
// the request body, and returning the response struct or an error.
type handlerFunc func(ctx context.Context, params map[string]string, query url.Values, body []byte) (res interface{}, err error)

// wrap adapts the handler function returning a standard http handler.
// It does tracing, metrics and response and error writing.
func wrap(endpoint string, handler handlerFunc) http.Handler {
	wrap := func(w http.ResponseWriter, r *http.Request) {
		defer observeAPILatency(endpoint)()

		ctx := r.Context()
		ctx = log.WithTopic(ctx, "vapi")
		ctx = log.WithCtx(ctx, z.Str("vapi_endpoint", endpoint))
		ctx = withCtxDuration(ctx)

		// TODO(corver): Add support for octet-stream (SSZ).
		contentType := r.Header.Get("Content-Type")
		if contentType != "" && !strings.Contains(contentType, "application/json") {
			writeError(ctx, w, endpoint, apiError{
				StatusCode: http.StatusUnsupportedMediaType,
				Message:    fmt.Sprintf("unsupported media type %s (only application/json supported)", contentType),
			})

			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeError(ctx, w, endpoint, err)
			return
		}

		res, err := handler(ctx, mux.Vars(r), r.URL.Query(), body)
		if err != nil {
			writeError(ctx, w, endpoint, err)
			return
		}

		writeResponse(ctx, w, endpoint, res)
	}

	return wrapTrace(endpoint, wrap)
}

// wrapTrace wraps the passed handler in a OpenTelemetry tracing span.
func wrapTrace(endpoint string, handler http.HandlerFunc) http.Handler {
	return otelhttp.NewHandler(handler, "core/validatorapi."+endpoint)
}

// writeResponse writes the 200 OK response and json response body.
func writeResponse(ctx context.Context, w http.ResponseWriter, endpoint string, response interface{}) {
	w.WriteHeader(http.StatusOK)

	if response == nil {
		return
	}

	b, err := json.Marshal(response)
	if err != nil {
		writeError(ctx, w, endpoint, errors.Wrap(err, "marshal response body"))
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if _, err = w.Write(b); err != nil {
		// Too late to also try to writeError at this point, so just log.
		log.Error(ctx, "Failed writing api response", err)
	}
}

// writeError writes a http json error response object.
func writeError(ctx context.Context, w http.ResponseWriter, endpoint string, err error) {
	if ctx.Err() != nil {
		// Client cancelled the request
		err = apiError{
			StatusCode: http.StatusRequestTimeout,
			Message:    "client cancelled request",
			Err:        ctx.Err(),
		}
	}

	var aerr apiError
	if !errors.As(err, &aerr) {
		aerr = apiError{
			StatusCode: http.StatusInternalServerError,
			Message:    "Internal server error",
			Err:        err,
		}
	}

	if aerr.StatusCode/100 == 4 {
		// 4xx status codes are client errors (not server), so log as debug only.
		log.Debug(ctx, "Validator api 4xx response",
			z.Int("status_code", aerr.StatusCode),
			z.Str("message", aerr.Message),
			z.Err(err),
			getCtxDuration(ctx))
	} else {
		// 5xx status codes (or other weird ranges) are server errors, so log as error.
		log.Error(ctx, "Validator api 5xx response", err,
			z.Int("status_code", aerr.StatusCode),
			z.Str("message", aerr.Message),
			getCtxDuration(ctx))
	}

	incAPIErrors(endpoint, aerr.StatusCode)

	res := errorResponse{
		Code:    aerr.StatusCode,
		Message: aerr.Message,
		// TODO(corver): Add support for debug mode error and stacktraces.
	}

	b, err2 := json.Marshal(res)
	if err2 != nil {
		// Log and continue to write nil b.
		log.Error(ctx, "Failed marshalling error response", err2)
	}

	w.WriteHeader(aerr.StatusCode)
	w.Header().Set("Content-Type", "application/json")

	if _, err2 = w.Write(b); err2 != nil {
		log.Error(ctx, "Failed writing api error", err2)
	}
}

// unmarshal parses the JSON-encoded request body and stores the result
// in the value pointed to by v.
func unmarshal(body []byte, v interface{}) error {
	if len(body) == 0 {
		return apiError{
			StatusCode: http.StatusBadRequest,
			Message:    "empty request body",
			Err:        errors.New("empty request body"),
		}
	}

	err := json.Unmarshal(body, v)
	if err != nil {
		return apiError{
			StatusCode: http.StatusBadRequest,
			Message:    "failed parsing request body",
			Err:        err,
		}
	}

	return nil
}

type durationKey struct{}

// withCtxDuration returns a copy of parent in which the current time is associated with the duration key.
func withCtxDuration(ctx context.Context) context.Context {
	return context.WithValue(ctx, durationKey{}, time.Now())
}

// getCtxDuration returns a zap field with the duration withCtxDuration was called on the context.
// Else it returns a noop zap field.
func getCtxDuration(ctx context.Context) z.Field {
	v := ctx.Value(durationKey{})
	if v == nil {
		return z.Skip
	}
	t0, ok := v.(time.Time)
	if !ok {
		return z.Skip
	}

	return z.Str("duration", time.Since(t0).String())
}

// hexQueryFixed parses a fixed length 0x-hex query parameter into target.
func hexQueryFixed(query url.Values, name string, target []byte) error {
	resp, ok, err := hexQuery(query, name)
	if err != nil {
		return err
	} else if !ok {
		return apiError{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("missing 0x-hex query parameter %s", name),
		}
	} else if len(resp) != len(target) {
		return apiError{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("invalid length for 0x-hex query parameter %s, expect %d bytes", name, len(target)),
		}
	}
	copy(target, resp)

	return nil
}

// hexQuery returns a 0x-prefixed hex query parameter with name or false if not present.
func hexQuery(query url.Values, name string) ([]byte, bool, error) {
	valueA, ok := query[name]
	if !ok || len(valueA) != 1 {
		return nil, false, nil
	}
	value := valueA[0]

	resp, err := hex.DecodeString(strings.TrimPrefix(value, "0x"))
	if err != nil {
		return nil, false, apiError{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("invalid 0x-hex query parameter %s [%s]", name, value),
			Err:        err,
		}
	}

	return resp, true, nil
}

// errorResponse an error response from the beacon-node api.
// See https://ethereum.github.io/beacon-APIs.
type errorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	// TODO(corver): Maybe add stacktraces field for debugging.
}
