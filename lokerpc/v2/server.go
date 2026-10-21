// Package lokerpc serves protobuf services through Connect and legacy HTTP RPC.
package lokerpc

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"connectrpc.com/connect"
	v1 "github.com/LOKE/pkg/lokerpc"
	"github.com/LOKE/pkg/requestid"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
)

// Meta extends legacy metadata with the service's protobuf descriptors.
type Meta struct {
	v1.Meta
	Proto        string `json:"proto,omitempty"`
	ProtoService string `json:"protoService,omitempty"`
}

// Service mounts a generated Connect handler under a required legacy Name.
type Service struct {
	Name    string
	Path    string
	Handler http.Handler
}

// MountHandlers mounts Connect paths, /rpc/<name>/<method>, and GET /rpc[/<name>] metadata.
func MountHandlers(logger log.Logger, mux v1.Mux, services []Service, legacy ...*v1.Service) error {
	names := make(map[string]bool)
	paths := make(map[string]bool)
	descriptors := make([]protoreflect.ServiceDescriptor, len(services))
	for _, service := range legacy {
		if service == nil {
			return fmt.Errorf("nil legacy service")
		}
		if err := reserveName(names, service.Name); err != nil {
			return err
		}
	}
	for i, service := range services {
		if err := reserveName(names, service.Name); err != nil {
			return err
		}
		if service.Handler == nil {
			return fmt.Errorf("service %q requires a handler", service.Name)
		}
		descriptor, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(strings.Trim(service.Path, "/")))
		if err != nil {
			return fmt.Errorf("resolve service %q: %w", service.Name, err)
		}
		serviceDescriptor, ok := descriptor.(protoreflect.ServiceDescriptor)
		if !ok || service.Path != "/"+string(serviceDescriptor.FullName())+"/" {
			return fmt.Errorf("service %q has invalid Connect path %q", service.Name, service.Path)
		}
		descriptors[i] = serviceDescriptor
		if paths[service.Path] {
			return fmt.Errorf("duplicate Connect path %q", service.Path)
		}
		paths[service.Path] = true
	}
	root := struct {
		Services []Meta `json:"services"`
	}{Services: []Meta{}}
	for _, service := range legacy {
		root.Services = append(root.Services, Meta{Meta: service.Meta()})
	}
	metadata := make([]http.Handler, len(services))
	for i, service := range services {
		meta, err := serviceMeta(service.Name, descriptors[i])
		if err != nil {
			return fmt.Errorf("service %q metadata: %w", service.Name, err)
		}
		root.Services = append(root.Services, meta)
		metadata[i], err = metaHandler(meta)
		if err != nil {
			return fmt.Errorf("encode service metadata: %w", err)
		}
	}
	sort.Slice(root.Services, func(i, j int) bool { return root.Services[i].ServiceName < root.Services[j].ServiceName })
	rootHandler, err := metaHandler(root)
	if err != nil {
		return fmt.Errorf("encode root metadata: %w", err)
	}
	for _, service := range legacy {
		service.MountHandlers(logger, mux)
	}
	for i, service := range services {
		mux.Handle(service.Path, service.Handler)
		mux.Handle("/rpc/"+service.Name+"/", bridge(logger, service, descriptors[i]))
		mux.Handle("/rpc/"+service.Name, metadata[i])
	}
	mux.Handle("/rpc", rootHandler)
	return nil
}

func reserveName(names map[string]bool, name string) error {
	valid := name != ""
	for i := range len(name) {
		c := name[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || i > 0 && (c == '_' || c == '.' || c == '-') {
			continue
		}
		valid = false
		break
	}
	if !valid {
		return fmt.Errorf("invalid legacy service name %q", name)
	}
	if names[name] {
		return fmt.Errorf("duplicate legacy service name %q", name)
	}
	names[name] = true
	return nil
}

func metaHandler(meta any) (http.Handler, error) {
	data, err := json.Marshal(meta)
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", v1.ContentType)
		_, _ = w.Write(data)
	}), nil
}

type errorBody struct {
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
	Type    string `json:"type,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", v1.ContentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func bridge(logger log.Logger, service Service, descriptor protoreflect.ServiceDescriptor) http.Handler {
	logger = log.With(logger, "rpc_service", service.Name)
	methods := make(map[string]protoreflect.MethodDescriptor)
	observers := make(map[string]func(string, time.Duration))
	for i := range descriptor.Methods().Len() {
		method := descriptor.Methods().Get(i)
		if !method.IsStreamingClient() && !method.IsStreamingServer() {
			name := protoMethodName(string(method.Name()))
			methods[name] = method
			observers[name] = v1.NewRequestObserver(service.Name + "." + name)
		}
	}
	prefix := "/rpc/" + service.Name + "/"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, errorBody{Message: "Method Not Allowed"})
			return
		}
		name := strings.TrimPrefix(r.URL.Path, prefix)
		method, ok := methods[name]
		if !ok {
			writeJSON(w, http.StatusNotFound, errorBody{Message: "Not Found"})
			return
		}
		start := time.Now()
		failure := ""
		failureMessage := ""
		defer func() {
			observers[name](failure, time.Since(start))
			if failure != "" {
				_ = level.Error(logger).Log("method", name, "err", failureMessage)
			}
		}()
		ctx := r.Context()
		if _, ok := requestid.FromContext(ctx); !ok {
			ctx = requestid.NewContextFromRequest(r)
		}
		if deadline, err := time.Parse(time.RFC3339Nano, r.Header.Get("X-Request-Deadline")); err == nil {
			var cancel context.CancelFunc
			ctx, cancel = context.WithDeadline(ctx, deadline)
			defer cancel()
		}
		encoding := r.Header.Get("Content-Encoding")
		if encoding != "" && encoding != "identity" && encoding != "gzip" {
			failure, failureMessage = "unimplemented", "Unsupported Content-Encoding"
			writeJSON(w, http.StatusUnsupportedMediaType, errorBody{Message: failureMessage})
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			failure, failureMessage = "invalid_argument", "Invalid JSON"
			writeJSON(w, http.StatusBadRequest, errorBody{Message: failureMessage})
			return
		}
		if (encoding == "" || encoding == "identity") && len(bytes.TrimSpace(body)) == 0 {
			body = []byte("{}")
		}
		forward := r.Clone(ctx)
		forward.URL.Path = service.Path + string(method.Name())
		forward.URL.RawPath = ""
		forward.Body = io.NopCloser(bytes.NewReader(body))
		forward.ContentLength = int64(len(body))
		forward.Header.Set("Content-Type", "application/json")
		forward.Header.Set("Connect-Protocol-Version", "1")
		forward.Header.Set("Accept-Encoding", "identity")
		response := newBufferedResponse()
		service.Handler.ServeHTTP(response, forward)
		responseBody, err := decodeBody(response.body.Bytes(), response.header.Get("Content-Encoding"))
		if err != nil {
			failure, failureMessage = "internal", err.Error()
			writeJSON(w, http.StatusInternalServerError, errorBody{Message: failureMessage})
			return
		}
		for name, values := range response.header {
			if name != "Content-Type" && name != "Content-Length" && name != "Content-Encoding" && name != "Accept-Encoding" && !strings.HasPrefix(name, "Trailer-") {
				w.Header()[name] = values
			}
		}
		if response.status != http.StatusOK {
			var wire struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(responseBody, &wire); err != nil {
				wire.Message = http.StatusText(response.status)
			}
			failure = wire.Code
			failureMessage = wire.Message
			if wire.Code == connect.CodeInvalidArgument.String() {
				// Connect has enforced its decompressed size limit before this validation.
				decoded, err := decodeBody(body, encoding)
				if err == nil {
					err = (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(decoded, dynamicpb.NewMessage(method.Input()))
				}
				if err != nil {
					failureMessage = "Invalid JSON"
					writeJSON(w, http.StatusBadRequest, errorBody{Message: failureMessage})
					return
				}
			}
			status := http.StatusBadRequest
			result := errorBody{Message: wire.Message, Code: wire.Code, Type: wire.Code}
			if wire.Code == "" || wire.Code == connect.CodeUnknown.String() || wire.Code == connect.CodeInternal.String() {
				status = http.StatusInternalServerError
				result.Code, result.Type = "", ""
				if failure == "" {
					failure = "unknown"
				}
			}
			writeJSON(w, status, result)
			return
		}
		result := dynamicpb.NewMessage(method.Output())
		err = protojson.Unmarshal(responseBody, result)
		if err == nil {
			body, err = (protojson.MarshalOptions{EmitDefaultValues: true}).Marshal(result)
		}
		if err != nil {
			failure, failureMessage = "internal", err.Error()
			writeJSON(w, http.StatusInternalServerError, errorBody{Message: err.Error()})
			return
		}
		w.Header().Set("Content-Type", v1.ContentType)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	})
}

func decodeBody(body []byte, encoding string) ([]byte, error) {
	if encoding != "gzip" {
		return body, nil
	}
	reader, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

type bufferedResponse struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func newBufferedResponse() *bufferedResponse {
	return &bufferedResponse{header: make(http.Header), status: http.StatusOK}
}
func (w *bufferedResponse) Header() http.Header            { return w.header }
func (w *bufferedResponse) WriteHeader(status int)         { w.status = status }
func (w *bufferedResponse) Write(body []byte) (int, error) { return w.body.Write(body) }
