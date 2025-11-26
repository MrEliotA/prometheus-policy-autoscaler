package metrics

import (
    "context"
    "fmt"
    "time"

    "github.com/prometheus/client_golang/api"
    v1 "github.com/prometheus/client_golang/api/prometheus/v1"
    "github.com/prometheus/common/model"
)

// Client is an interface to keep the reconciler easy to test.
// In unit tests you can replace this with a fake implementation.
type Client interface {
    // QueryVector executes a PromQL expression and returns a single float64 value.
    // We intentionally constrain ourselves to "scalar-like" queries to keep
    // semantics simple and predictable.
    QueryVector(ctx context.Context, promql string) (float64, error)
}

// HTTPClient implements Client using the Prometheus HTTP API.
type HTTPClient struct {
    api v1.API
}

// NewHTTPClient builds a new Client for the given Prometheus base URL.
func NewHTTPClient(baseURL string) (Client, error) {
    cfg := api.Config{
        Address: baseURL,
    }

    c, err := api.NewClient(cfg)
    if err != nil {
        return nil, fmt.Errorf("creating prometheus client: %w", err)
    }

    return &HTTPClient{
        api: v1.NewAPI(c),
    }, nil
}

// QueryVector implements the Client interface using the v1 API.
func (c *HTTPClient) QueryVector(ctx context.Context, promql string) (float64, error) {
    // NOTE: In tests you may want to inject "now" for determinism.
    result, warnings, err := c.api.Query(ctx, promql, time.Now())
    if err != nil {
        return 0, fmt.Errorf("prometheus query failed: %w", err)
    }

    if len(warnings) > 0 {
        // We intentionally don't treat warnings as fatal here.
        // They should be logged at call sites for observability.
    }

    switch v := result.(type) {
    case model.Vector:
        if len(v) == 0 {
            return 0, fmt.Errorf("prometheus query returned empty vector")
        }
        return float64(v[0].Value), nil
    case *model.Scalar:
        return float64(v.Value), nil
    default:
        return 0, fmt.Errorf("unexpected prometheus result type %T", v)
    }
}
