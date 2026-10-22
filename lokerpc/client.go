package lokerpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"reflect"
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

func NewClient(baseURL string, opts ...ClientOption) Client {
	c := Client{bURL: normalizeBaseURL(baseURL), client: http.DefaultClient}
	for _, opt := range opts {
		opt(&c)
	}
	return c
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

func (e *rpcClientError) ErrorCode() string {
	return e.Code
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

	codecs  []Codec
	session *clientSession
}

func normalizeBaseURL(baseURL string) string {
	return strings.TrimRight(baseURL, "/") + "/"
}

func (c Client) DoRequest(ctx context.Context, method string, args, result any) (finalErr error) {
	url := c.bURL + method

	defer prometheus.NewTimer(clientLatency.WithLabelValues(c.bURL, method)).ObserveDuration()
	clientRequestCount.WithLabelValues(c.bURL, method).Inc()

	var session *negotiated
	if c.session != nil {
		session = c.session.negotiate(ctx, c.client, strings.TrimSuffix(c.bURL, "/"), c.codecs)
	}

	res, err := c.send(ctx, url, method, args, session)
	if err == nil && session != nil && session.codec != nil && binaryRejected(res) {
		res.Body.Close()
		c.session.invalidate(session)
		session = nil
		res, err = c.send(ctx, url, method, args, nil)
	}

	defer func() {
		if finalErr == nil {
			return
		}

		errType := "unknown"
		if rpcErr, ok := errors.AsType[*rpcClientError](finalErr); ok {
			errType = rpcErr.Type
		} else if _, ok := finalErr.(*json.InvalidUnmarshalError); ok {
			errType = "json_decode_error"
		} else if _, ok := finalErr.(*json.SyntaxError); ok {
			errType = "json_decode_error"
		} else if finalErr == context.Canceled {
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
		if result == nil {
			return nil
		}
		if codec := findCodec(c.codecs, res.Header.Get("Content-Type")); codec != nil {
			return c.decodeBinary(codec, res.Body, method, result, session)
		}
		if res.ContentLength == 0 {
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

// send performs one POST, using the negotiated codec when the method's schemas
// are known and plain JSON otherwise.
func (c Client) send(ctx context.Context, url, method string, args any, session *negotiated) (*http.Response, error) {
	endpoint, known := session.endpoint(method)

	var body bytes.Buffer
	contentType := "application/json"
	if known && endpoint.RequestTypeDef != nil {
		encoded, err := session.codec.Marshal(args, endpoint.RequestTypeDef, session.defs)
		if err != nil {
			return nil, err
		}
		body.Write(encoded)
		contentType = session.codec.ContentType()
	} else {
		err := json.NewEncoder(&body).Encode(args)
		if err != nil {
			return nil, err
		}
	}

	req, err := http.NewRequest("POST", url, &body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", contentType)

	if known {
		req.Header.Set("If-Match", session.etag)
		if endpoint.ResponseTypeDef != nil {
			req.Header.Set("Accept", session.codec.ContentType()+", application/json;q=0.5")
		}
	}

	if reqID, ok := requestid.FromContext(ctx); ok {
		req.Header.Set("X-Request-ID", reqID.String())
	}

	if deadline, ok := ctx.Deadline(); ok {
		// Could probably also use .Format(time.RFC3339Nano), but MarshalJSON
		// seems to do more, and I think it'll be safer for JS
		b, err := deadline.MarshalJSON()
		if err == nil {
			// string(b[1:len(b)-1]) strips the quotes from the value
			req.Header.Set("X-Request-Deadline", string(b[1:len(b)-1]))
		}
	}

	return c.client.Do(req.WithContext(ctx))
}

func (c Client) decodeBinary(codec Codec, body io.Reader, method string, result any, session *negotiated) error {
	endpoint, known := session.endpoint(method)
	if !known || endpoint.ResponseTypeDef == nil {
		return fmt.Errorf("Error decoding rpc response: unexpected %s response", codec.ContentType())
	}

	data, err := io.ReadAll(body)
	if err != nil {
		return fmt.Errorf("Error reading rpc response: %v", err)
	}
	if len(data) == 0 {
		// An empty body is a nil result, like JSON null.
		if target := reflect.ValueOf(result); target.Kind() == reflect.Pointer && !target.IsNil() {
			target.Elem().SetZero()
		}
		return nil
	}

	err = codec.Unmarshal(data, result, endpoint.ResponseTypeDef, session.defs)
	if err != nil {
		return fmt.Errorf("Error decoding rpc response: %v", err)
	}
	return nil
}

// binaryRejected reports a 412, or a 400/415 that is not JSON, which is how a
// replica without codec support (e.g. mid rolling deploy) answers a binary body.
func binaryRejected(res *http.Response) bool {
	switch res.StatusCode {
	case http.StatusPreconditionFailed:
		return true
	case http.StatusBadRequest, http.StatusUnsupportedMediaType:
		mediaType, _, err := mime.ParseMediaType(res.Header.Get("Content-Type"))
		return err != nil || mediaType != "application/json"
	}
	return false
}
