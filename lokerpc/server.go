package lokerpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	jtd "github.com/jsontypedef/json-typedef-go"
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

type Failer interface {
	Failed() error
}

// Service
type Service struct {
	Name string
	Help string

	endpointCodecs EndpointCodecMap
}

// NewService creates a new Service
func NewService(name, help string, ecm EndpointCodecMap) *Service {
	return &Service{
		Name:           name,
		Help:           help,
		endpointCodecs: ecm,
	}
}

// Endpoint is an abstract rpc endpoint
type Endpoint func(ctx context.Context, request interface{}) (response interface{}, err error)

// EndpointCodec defines a server Endpoint and its associated codecs
type EndpointCodec struct {
	Endpoint   Endpoint
	Decode     DecodeRequestFunc
	Help       string
	ParamNames []string

	requestType      reflect.Type
	responseType     reflect.Type
	errOnNilResponse bool
}

// EndpointCodecMap maps the Request.Method to the proper EndpointCodec
type EndpointCodecMap map[string]EndpointCodec

type Meta struct {
	ServiceName string                `json:"serviceName"`
	MultiArg    bool                  `json:"multiArg"`
	Help        string                `json:"help"`
	Interfaces  []EndpointMeta        `json:"interfaces"`
	Definitions map[string]jtd.Schema `json:"definitions,omitempty"`
}

type EndpointMeta struct {
	MethodName      string      `json:"methodName"`
	ParamNames      []string    `json:"paramNames"`
	MethodTimeout   int         `json:"methodTimeout"`
	Help            string      `json:"help"`
	RequestTypeDef  *jtd.Schema `json:"requestTypeDef,omitempty"`
	ResponseTypeDef *jtd.Schema `json:"responseTypeDef,omitempty"`
}

type RootMeta struct {
	Services []*Meta `json:"services"`
}

type standardResponse struct {
	Res any
	Err error
}

func (r standardResponse) Result() any   { return r.Res }
func (r standardResponse) Failed() error { return r.Err }

func DecodeRequest[Req any](_ context.Context, msg json.RawMessage) (any, error) {
	var req Req
	err := json.Unmarshal(msg, &req)
	if err != nil {
		return nil, err
	}
	return req, nil
}

type StandardMethod[Req any, Res any] func(context.Context, Req) (Res, error)

func MakeStandardEndpoint[Req any, Res any](method StandardMethod[Req, Res]) Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(Req)
		res, err := method(ctx, req)
		return standardResponse{res, err}, nil
	}
}

type EndpointCodecOption func(*EndpointCodec)

// MakeStandardEndpointCodec
func MakeStandardEndpointCodec[Req any, Res any](method StandardMethod[Req, Res], help string, opts ...EndpointCodecOption) EndpointCodec {
	var req Req
	var res Res

	ec := EndpointCodec{
		Endpoint:   MakeStandardEndpoint(method),
		Decode:     DecodeRequest[Req],
		ParamNames: FieldNames(req),
		Help:       help,

		requestType:  reflect.TypeOf(req),
		responseType: reflect.TypeOf(res),
	}

	for _, opt := range opts {
		opt(&ec)
	}

	return ec
}

func NoNilResponse() EndpointCodecOption {
	return func(ec *EndpointCodec) {
		ec.errOnNilResponse = true
	}
}

// NewServer constructs a new server, which implements http.Handler.
//
// Deprecated: Use the MountHandlers with Services instead
func NewServer(serviceName string, ecm EndpointCodecMap, logger log.Logger) http.Handler {
	ecm = wrapMetrics(serviceName, ecm)
	mux := http.NewServeMux()
	meta := Meta{
		ServiceName: serviceName,
		MultiArg:    false,
		Help:        "",
	}

	for methodName, ec := range ecm {
		l := log.With(logger, "rpc_service", serviceName, "method", methodName)

		mux.HandleFunc("/"+methodName, makeHandler(l, ec))
		meta.Interfaces = append(meta.Interfaces, EndpointMeta{
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

type Mux interface {
	Handle(pattern string, handler http.Handler)
}

// MountHandlers mounts all the service endpoints onto the mux
// Endpoints are mounted at
//
//	POST /rpc/<service>/<method>
//
// Meta data is serviced from
//
//	GET /rpc
//	GET /rpc/<service>
func MountHandlers(logger log.Logger, mux Mux, services ...*Service) {
	rootmeta := RootMeta{}

	for _, service := range services {
		ecm := wrapMetrics(service.Name, service.endpointCodecs)

		defs := map[reflect.Type]*NamedSchema{}

		meta := &Meta{
			ServiceName: service.Name,
			MultiArg:    false,
			Help:        service.Help,
		}

		for methodName, ec := range ecm {
			l := log.With(logger, "rpc_service", service.Name, "method", methodName)

			mux.Handle("/rpc/"+service.Name+"/"+methodName, makeHandler(l, ec))

			endMeta := EndpointMeta{
				MethodName:    methodName,
				MethodTimeout: 60000,
				Help:          ec.Help,
				ParamNames:    ec.ParamNames,
			}

			if ec.requestType != nil {
				endMeta.RequestTypeDef = TypeSchema(ec.requestType, defs)
				endMeta.RequestTypeDef.Nullable = false
			}
			if ec.responseType != nil {
				endMeta.ResponseTypeDef = TypeSchema(ec.responseType, defs)
				if ec.errOnNilResponse {
					endMeta.ResponseTypeDef.Nullable = false
				}
			}

			meta.Interfaces = append(meta.Interfaces, endMeta)
		}

		meta.Definitions = TypeDefs(defs)

		// service meta endpoint
		mux.Handle("/rpc/"+service.Name, newMetaHandler(meta))

		rootmeta.Services = append(rootmeta.Services, meta)
	}

	// root meta endpoint
	mux.Handle("/rpc", newMetaHandler(rootmeta))
}

func newMetaHandler(meta any) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			rw.Header().Set("Content-Type", ContentType)
			rw.WriteHeader(http.StatusOK)
			if err := json.NewEncoder(rw).Encode(meta); err != nil {
				panic(err)
			}
		} else {
			http.Error(rw, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func FieldNames(i interface{}) []string {
	pm := []string{}
	t := reflect.TypeOf(i)

	for n := 0; n < t.NumField(); n++ {
		f := t.Field(n)
		name, _ := parseTag(f.Tag.Get("json"))
		if name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		pm = append(pm, name)
	}
	return pm
}

// Taken from encoding/json/tags.go
func parseTag(tag string) (string, bool) {
	if idx := strings.Index(tag, ","); idx != -1 {
		return tag[:idx], tag[idx+1:] == "omitempty"
	}
	return tag, false
}

func wrapMetrics(serviceName string, ecm EndpointCodecMap) EndpointCodecMap {
	newECM := EndpointCodecMap{}

	for methodName, ec := range ecm {
		handlerName := serviceName + "." + methodName

		wec := ec
		wec.Endpoint = wrapEndpoint(handlerName, ec.Endpoint)

		newECM[methodName] = wec
	}

	return newECM
}

func wrapEndpoint(handlerName string, e Endpoint) Endpoint {
	c := count.WithLabelValues(handlerName)
	l := latency.WithLabelValues(handlerName)

	return func(ctx context.Context, request interface{}) (result interface{}, err error) {
		t := prometheus.NewTimer(l)
		defer t.ObserveDuration()
		defer func() {
			if err != nil {
				failures.WithLabelValues(handlerName, "unknown").Inc()
			} else if e, ok := result.(Failer); ok && e.Failed() != nil {
				failures.WithLabelValues(handlerName, "unknown").Inc()
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

		if e, ok := result.(Failer); ok && e.Failed() != nil {
			logErr("err", e.Failed())

			status = http.StatusBadRequest
			result = struct {
				Message string `json:"message"`
			}{e.Failed().Error()}
		} else {
			if r, ok := result.(Resulter); ok {
				result = r.Result()
			}

			if result == nil && ec.errOnNilResponse {
				logErr("err", "unexpected nil response")

				status = http.StatusInternalServerError
				result = struct {
					Message string `json:"message"`
				}{"unexpected nil response"}
			}
		}

		w.Header().Set("Content-Type", ContentType)
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(result)
	}
}

func writeBadReq(w http.ResponseWriter, format string, a ...interface{}) {
	http.Error(w, fmt.Sprintf(format+"\n", a...), http.StatusBadRequest)
}
