package lokerpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/go-kit/kit/endpoint"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	// ContentType defines the content type to be served.
	ContentType = "application/json; charset=utf-8"
)

var latency *prometheus.HistogramVec = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name: "http_rpc_request_duration_seconds",
	Help: "Duration of rpc requests",
}, []string{"handler"})

var count *prometheus.CounterVec = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "http_rpc_requests_total",
	Help: "The total number of rpc requests received",
}, []string{"handler"})

var failures *prometheus.CounterVec = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "http_rpc_failures_total",
	Help: "The total number of rpc failures received",
}, []string{"handler", "type"})

// should really avoid init,
// works for now
func init() {
	prometheus.MustRegister(latency)
	prometheus.MustRegister(count)
	prometheus.MustRegister(failures)
}

type DecodeRequestFunc func(context.Context, json.RawMessage) (request interface{}, err error)
type EncodeResponseFunc func(context.Context, interface{}) (response json.RawMessage, err error)

type Resulter interface {
	Result() interface{}
}

// EndpointCodec defines a server Endpoint and its associated codecs
type EndpointCodec struct {
	Endpoint   endpoint.Endpoint
	Decode     DecodeRequestFunc
	Help       string
	ParamNames []string
}

// EndpointCodecMap maps the Request.Method to the proper EndpointCodec
type EndpointCodecMap map[string]EndpointCodec

type meta struct {
	ServiceName string         `json:"serviceName"`
	MultiArg    bool           `json:"multiArg"`
	Help        string         `json:"help"`
	Interfaces  []endpointMeta `json:"interfaces"`
}

type endpointMeta struct {
	MethodName    string   `json:"methodName"`
	ParamNames    []string `json:"paramNames"`
	MethodTimeout int      `json:"methodTimeout"`
	Help          string   `json:"help"`
}

// NewServer constructs a new server, which implements http.Handler.
func NewServer(serviceName string, ecm EndpointCodecMap, logger log.Logger) http.Handler {
	ecm = wrapMetrics(serviceName, ecm)
	mux := http.NewServeMux()
	meta := meta{
		ServiceName: serviceName,
		MultiArg:    false,
		Help:        "",
	}

	for methodName, ec := range ecm {
		l := log.With(logger, "rpc_service", serviceName, "method", methodName)

		mux.HandleFunc("/"+methodName, makeHandler(l, ec))
		meta.Interfaces = append(meta.Interfaces, endpointMeta{
			MethodName:    methodName,
			MethodTimeout: 60000,
			Help:          ec.Help,
			ParamNames:    ec.ParamNames,
		})
	}

	// encode the metadata to json
	var metab []byte
	{
		b := &bytes.Buffer{}
		json.NewEncoder(b).Encode(meta)
		metab = b.Bytes()
	}

	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/", "":
			if r.Method == "GET" {
				if _, err := rw.Write(metab); err != nil {
					panic(err)
				}
			} else {
				http.Error(rw, "Method not allowed", http.StatusMethodNotAllowed)
			}
		default:
			mux.ServeHTTP(rw, r)
		}
	})
}

func FieldNames(i interface{}) []string {
	pm := []string{}
	t := reflect.TypeOf(i)

	for n := 0; n < t.NumField(); n++ {
		f := t.Field(n)
		name := parseTag(f.Tag.Get("json"))
		if name == "" {
			name = f.Name
		}
		pm = append(pm, name)
	}
	return pm

}

// Taken from encoding/json/tags.go
func parseTag(tag string) string {
	if idx := strings.Index(tag, ","); idx != -1 {
		return tag[:idx]
	}
	return tag
}

func wrapMetrics(serviceName string, ecm EndpointCodecMap) EndpointCodecMap {
	newECM := EndpointCodecMap{}

	for methodName, ec := range ecm {
		handlerName := serviceName + "." + methodName
		newECM[methodName] = EndpointCodec{
			Endpoint:   wrapEndpoint(handlerName, ec.Endpoint),
			Decode:     ec.Decode,
			Help:       ec.Help,
			ParamNames: ec.ParamNames,
		}
	}

	return newECM
}

func wrapEndpoint(handlerName string, e endpoint.Endpoint) endpoint.Endpoint {
	c := count.WithLabelValues(handlerName)
	l := latency.WithLabelValues(handlerName)

	return func(ctx context.Context, request interface{}) (result interface{}, err error) {
		t := prometheus.NewTimer(l)
		defer t.ObserveDuration()
		defer func() {
			if err != nil {
				failures.WithLabelValues(handlerName, "unknown")
			}
		}()
		c.Inc()
		return e(ctx, request)
	}
}

func makeHandler(logger log.Logger, ec EndpointCodec) http.HandlerFunc {
	logErr := level.Error(logger).Log
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "405 must POST", http.StatusMethodNotAllowed)
			return
		}
		ctx := r.Context()

		// Decode the body into an  object
		var jsonParams json.RawMessage
		err := json.NewDecoder(r.Body).Decode(&jsonParams)
		if err != nil {
			writeBadReq(w, "JSON could not be decoded: %v", err)
			return
		}

		// Decode the JSON "params"
		reqParams, err := ec.Decode(ctx, jsonParams)
		if err != nil {
			writeBadReq(w, "Invalid request: %v", err)
			return
		}

		// Call the Endpoint with the params
		result, err := ec.Endpoint(ctx, reqParams)
		if err != nil {
			logErr("msg", "endpoint error", "err", err)
			writeBadReq(w, "Endpoint error: %v", err)
			return
		}

		status := http.StatusOK
		if r, ok := result.(Resulter); ok {
			result = r.Result()
		}

		if e, ok := result.(endpoint.Failer); ok && e.Failed() != nil {
			logErr("err", e.Failed())

			status = http.StatusBadRequest
			result = struct {
				Message string `json:"message"`
			}{e.Failed().Error()}
		}

		w.Header().Set("Content-Type", ContentType)
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(result)
	}
}

func writeBadReq(w http.ResponseWriter, format string, a ...interface{}) {
	http.Error(w, fmt.Sprintf(format+"\n", a...), http.StatusBadRequest)
}
