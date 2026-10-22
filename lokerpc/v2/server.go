// Package lokerpc serves protobuf services through Connect and legacy HTTP RPC.
package lokerpc

//go:generate protoc -I proto --plugin=protoc-gen-go=./protoc-gen-go.sh --go_out=. --go_opt=module=github.com/LOKE/pkg/lokerpc/v2 --go_opt=Mloke/rpc/v1/error.proto=github.com/LOKE/pkg/lokerpc/v2/rpcv1 --go_opt=Mloke/rpc/v1/descriptor.proto=github.com/LOKE/pkg/lokerpc/v2/rpcv1 proto/loke/rpc/v1/error.proto proto/loke/rpc/v1/descriptor.proto

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
	"sync"
	"time"

	"connectrpc.com/connect"
	v1 "github.com/LOKE/pkg/lokerpc"
	"github.com/LOKE/pkg/lokerpc/v2/rpcv1"
	"github.com/LOKE/pkg/requestid"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
)

// Meta describes a service using legacy-compatible JTD schemas.
type Meta struct {
	v1.Meta
	derived bool
}

// Service builds a generated Connect handler under a required legacy Name.
type Service struct {
	Name string
	// Handler must prepend the supplied options before service-specific options.
	// It is called separately for native and bridge codecs. Keep registration
	// side effects outside this factory and share stateful interceptor instances.
	Handler func(...connect.HandlerOption) (string, http.Handler)
	// NoBridge disables legacy method routes so a same-name v1 service can keep them.
	NoBridge bool
}

// MountHandlers mounts Connect, metadata, and bridges unless NoBridge is set.
// Only a NoBridge proto service and one v1 service may share a name.
func MountHandlers(logger log.Logger, mux v1.Mux, services []Service, legacy ...*v1.Service) error {
	legacyIndexes, protoNames, err := validateServiceNames(services, legacy)
	if err != nil {
		return err
	}
	paths := make(map[string]bool)
	descriptors := make([]protoreflect.ServiceDescriptor, len(services))
	handlers := make([]http.Handler, len(services))
	bridgeHandlers := make([]http.Handler, len(services))
	servicePaths := make([]string, len(services))
	for i, service := range services {
		if service.Handler == nil {
			return fmt.Errorf("service %q requires a handler", service.Name)
		}
		path, handler := service.Handler(connect.WithInterceptors(errorInterceptor{}))
		if handler == nil {
			return fmt.Errorf("service %q requires a handler", service.Name)
		}
		servicePaths[i], handlers[i] = path, handler
		if !service.NoBridge {
			bridgePath, bridgeHandler := service.Handler(connect.WithInterceptors(errorInterceptor{}), connect.WithCodec(legacyJSONCodec{}))
			if bridgeHandler == nil || bridgePath != path {
				return fmt.Errorf("service %q requires a matching bridge handler", service.Name)
			}
			bridgeHandlers[i] = bridgeHandler
		}
		descriptor, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(strings.Trim(path, "/")))
		if err != nil {
			return fmt.Errorf("resolve service %q: %w", service.Name, err)
		}
		serviceDescriptor, ok := descriptor.(protoreflect.ServiceDescriptor)
		if !ok || path != "/"+string(serviceDescriptor.FullName())+"/" {
			return fmt.Errorf("service %q has invalid Connect path %q", service.Name, path)
		}
		descriptors[i] = serviceDescriptor
		if paths[path] {
			return fmt.Errorf("duplicate Connect path %q", path)
		}
		paths[path] = true
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
		if index, paired := legacyIndexes[service.Name]; paired {
			meta.Meta = root.Services[index].Meta
			meta.derived = false
			root.Services[index] = meta
		} else {
			root.Services = append(root.Services, meta)
		}
		files, err := descriptorSet(descriptors[i].ParentFile())
		if err != nil {
			return fmt.Errorf("service %q descriptors: %w", service.Name, err)
		}
		metadata[i], err = serviceMetaHandler(meta, &rpcv1.ServiceDescriptor{
			Service: string(descriptors[i].FullName()), Files: files,
		})
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
		service.MountHandlers(logger, methodMux{
			Mux: mux, metadataPath: "/rpc/" + service.Name, paired: protoNames[service.Name],
		})
	}
	for i, service := range services {
		mux.Handle(servicePaths[i], handlers[i])
		if !service.NoBridge {
			mux.Handle("/rpc/"+service.Name+"/", bridge(logger, service.Name, servicePaths[i], bridgeHandlers[i], descriptors[i]))
		}
		mux.Handle("/rpc/"+service.Name, metadata[i])
	}
	mux.Handle("/rpc", rootHandler)
	return nil
}

type methodMux struct {
	v1.Mux
	metadataPath string
	paired       bool
}

// v2 owns negotiated metadata; v1 retains ownership of its method handlers.
func (mux methodMux) Handle(path string, handler http.Handler) {
	if path == mux.metadataPath {
		if mux.paired {
			return
		}
		original := handler
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("Vary", "Accept")
			original.ServeHTTP(w, r)
		})
	}
	mux.Mux.Handle(path, handler)
}

func validateServiceNames(services []Service, legacy []*v1.Service) (map[string]int, map[string]bool, error) {
	names := make(map[string]bool)
	protoNames := make(map[string]bool)
	legacyIndexes := make(map[string]int)
	for i, service := range legacy {
		if service == nil {
			return nil, nil, fmt.Errorf("nil legacy service")
		}
		if err := reserveName(names, service.Name); err != nil {
			return nil, nil, err
		}
		legacyIndexes[service.Name] = i
	}
	for _, service := range services {
		if err := reserveName(protoNames, service.Name); err != nil {
			return nil, nil, err
		}
		if names[service.Name] && !service.NoBridge {
			return nil, nil, fmt.Errorf("duplicate legacy service name %q", service.Name)
		}
	}
	return legacyIndexes, protoNames, nil
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

var legacyRequestBuffers = sync.Pool{New: func() any { return new(bytes.Buffer) }}

func bridge(logger log.Logger, serviceName, servicePath string, handler http.Handler, descriptor protoreflect.ServiceDescriptor) http.Handler {
	logger = log.With(logger, "rpc_service", serviceName)
	methods := make(map[string]protoreflect.MethodDescriptor)
	observers := make(map[string]func(string, time.Duration))
	for i := range descriptor.Methods().Len() {
		method := descriptor.Methods().Get(i)
		if !method.IsStreamingClient() && !method.IsStreamingServer() {
			name := protoMethodName(string(method.Name()))
			methods[name] = method
			observers[name] = v1.NewRequestObserver(serviceName + "." + name)
		}
	}
	prefix := "/rpc/" + serviceName + "/"
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
		requestBuffer := legacyRequestBuffers.Get().(*bytes.Buffer)
		defer func() {
			requestBuffer.Reset()
			// Match Connect's upper bound for recyclable buffers.
			if requestBuffer.Cap() <= 8<<20 {
				legacyRequestBuffers.Put(requestBuffer)
			}
		}()
		_, err := io.Copy(requestBuffer, r.Body)
		body := requestBuffer.Bytes()
		if err != nil {
			failure, failureMessage = "invalid_argument", "Invalid JSON"
			writeJSON(w, http.StatusBadRequest, errorBody{Message: failureMessage})
			return
		}
		if (encoding == "" || encoding == "identity") && len(bytes.TrimSpace(body)) == 0 {
			body = []byte("{}")
		}
		forward := r.Clone(ctx)
		forward.URL.Path = servicePath + string(method.Name())
		forward.URL.RawPath = ""
		forward.Body = io.NopCloser(bytes.NewReader(body))
		forward.ContentLength = int64(len(body))
		forward.Header.Set("Content-Type", "application/loke-legacy-json")
		forward.Header.Set("Connect-Protocol-Version", "1")
		forward.Header.Set("Accept-Encoding", "identity")
		response := &bridgeResponse{destination: w, header: make(http.Header)}
		handler.ServeHTTP(response, forward)
		if response.status == 0 {
			response.status = http.StatusOK
		}
		if response.streamed {
			return
		}
		responseBody, err := decodeBody(response.body.Bytes(), response.header.Get("Content-Encoding"))
		if err != nil {
			failure, failureMessage = "internal", err.Error()
			writeJSON(w, http.StatusInternalServerError, errorBody{Message: failureMessage})
			return
		}
		copyBridgeHeaders(w, response.header)
		if response.status == http.StatusOK && len(responseBody) == 0 {
			if err := protojson.Unmarshal(responseBody, dynamicpb.NewMessage(method.Output())); err != nil {
				failure, failureMessage = "internal", err.Error()
				writeJSON(w, http.StatusInternalServerError, errorBody{Message: failureMessage})
				return
			}
		}
		if response.status != http.StatusOK {
			var wire struct {
				Code    string `json:"code"`
				Message string `json:"message"`
				Details []struct {
					Type  string `json:"type"`
					Value string `json:"value"`
				} `json:"details"`
			}
			if err := json.Unmarshal(responseBody, &wire); err != nil {
				wire.Message = http.StatusText(response.status)
			}
			failure = wire.Code
			failureMessage = wire.Message
			for _, detail := range wire.Details {
				if detail.Type == errorDetailType {
					if value, ok := decodeErrorDetail(detail.Value); ok {
						writeLegacyError(w, wire.Message, value)
						return
					}
				}
			}
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
		w.Header().Set("Content-Type", v1.ContentType)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(responseBody)
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

// bridgeResponse forwards successful identity responses without copying their
// body. Errors still need translation, and compressed responses still need
// decoding, so both retain the buffered path.
type bridgeResponse struct {
	destination http.ResponseWriter
	header      http.Header
	body        bytes.Buffer
	status      int
	streamed    bool
}

func (w *bridgeResponse) Header() http.Header { return w.header }

func (w *bridgeResponse) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
}

func (w *bridgeResponse) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	if !w.streamed && len(body) > 0 && w.status == http.StatusOK {
		encoding := w.header.Get("Content-Encoding")
		if encoding == "" || encoding == "identity" {
			w.streamed = true
			copyBridgeHeaders(w.destination, w.header)
			w.destination.Header().Set("Content-Type", v1.ContentType)
			w.destination.WriteHeader(w.status)
		}
	}
	if w.streamed {
		return w.destination.Write(body)
	}
	return w.body.Write(body)
}

func copyBridgeHeaders(w http.ResponseWriter, header http.Header) {
	for name, values := range header {
		if name != "Content-Type" && name != "Content-Length" && name != "Content-Encoding" && name != "Accept-Encoding" && !strings.HasPrefix(name, "Trailer-") {
			w.Header()[name] = values
		}
	}
}
