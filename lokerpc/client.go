package lokerpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/LOKE/pkg/errors"
	"github.com/LOKE/pkg/requestid"
	"github.com/prometheus/client_golang/prometheus"
)

var clientLatency *prometheus.HistogramVec = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name: "http_rpc_client_request_duration_seconds",
	Help: "Duration of rpc requests from the client",
}, []string{"service", "method"})

var clientRequestCount *prometheus.CounterVec = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "http_rpc_client_requests_total",
	Help: "The total number of rpc requests from the client",
}, []string{"service", "method"})

var clientFailures *prometheus.CounterVec = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "http_rpc_client_failures_total",
	Help: "The total number of rpc failures received",
}, []string{"service", "method", "type", "status_code"})

func init() {
	prometheus.MustRegister(clientLatency)
	prometheus.MustRegister(clientRequestCount)
	prometheus.MustRegister(clientFailures)
}

func NewClient(baseURL string) Client {
	return newClientWithClient(baseURL, http.DefaultClient)
}

func newClientWithClient(baseURL string, client *http.Client) Client {
	return Client{bURL: normalizeBaseURL(baseURL), client: client}
}

// NOTE: Maybe this should be exported, leaving it for now -- Dom
// Could also make this a map[string]any for passing through arbitrary fields
// to the upstream caller
type rpcClientError struct {
	Message   string `json:"message"`
	Instance  string `json:"instance,omitempty"`
	Expose    bool   `json:"expose,omitempty"`
	Code      string `json:"code,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Type      string `json:"type,omitempty"`
}

func (e *rpcClientError) ErrorID() string {
	return e.Instance
}

func (e *rpcClientError) ErrorType() string {
	return e.Type
}

func (e *rpcClientError) Public() bool {
	return e.Expose
}

func (e *rpcClientError) Error() string {
	return e.Message
	// return fmt.Sprintf("RPC Error response: %s", e.Message)
}

type Client struct {
	bURL   string
	client *http.Client
}

func normalizeBaseURL(baseURL string) string {
	return strings.TrimRight(baseURL, "/") + "/"
}

func (c Client) DoRequest(ctx context.Context, method string, args, result any) (finalErr error) {
	b := new(bytes.Buffer)
	if err := json.NewEncoder(b).Encode(args); err != nil {
		return err
	}

	url := c.bURL + method
	req, err := http.NewRequest("POST", url, b)
	if err != nil {
		return err
	}

	defer prometheus.NewTimer(clientLatency.WithLabelValues(c.bURL, method)).ObserveDuration()
	clientRequestCount.WithLabelValues(c.bURL, method).Inc()

	req.Header.Set("Content-Type", "application/json")

	if reqID, ok := requestid.FromContext(ctx); ok {
		req.Header.Set("X-Request-ID", reqID.String())
	}

	if deadline, ok := ctx.Deadline(); ok {
		// Could probably also use .Format(time.RFC3339Nano), but MarshalJSON
		// seems to do more, and I think it'll be safer for JS
		b, err := deadline.MarshalJSON()
		if err != nil {
			// string(b[1:len(b)-1]) strips the quotes from the value
			req.Header.Set("X-Request-Deadline", string(b[1:len(b)-1]))
		}
	}

	req = req.WithContext(ctx)
	res, err := c.client.Do(req)

	defer func() {
		if finalErr == nil {
			return
		}

		errType := "unknown"
		if rpcErr, ok := errors.AsType[*rpcClientError](err); ok {
			errType = rpcErr.Type
		} else if errors.Is(err, &json.InvalidUnmarshalError{}) {
			errType = "json_decode_error"
		} else if err == context.Canceled {
			errType = "aborted"
		}

		status := "-1"
		if res != nil {
			status = strconv.Itoa(res.StatusCode)
		}

		clientFailures.WithLabelValues(c.bURL, method, errType, status).Inc()
	}()

	if err != nil {
		return err
	}
	defer res.Body.Close()

	switch res.StatusCode {
	case http.StatusOK:
		if res.ContentLength == 0 || result == nil {
			return nil
		}
		if err := json.NewDecoder(res.Body).Decode(result); err != nil {
			return fmt.Errorf("Error decoding rpc response: %v", err)
		}
	case http.StatusNoContent:
		return nil
	case http.StatusNotFound:
		return fmt.Errorf("Error rpc method not found: %v", url)
	default:
		err := &rpcClientError{}
		jsonErr := json.NewDecoder(res.Body).Decode(err)
		if jsonErr != nil {
			return fmt.Errorf("Error decoding rpc error response: %v", jsonErr)
		}
		return err
	}
	return nil
}
