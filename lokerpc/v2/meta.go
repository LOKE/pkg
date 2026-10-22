package lokerpc

import (
	"encoding/json"
	"mime"
	"net/http"
	"strconv"
	"strings"

	v1 "github.com/LOKE/pkg/lokerpc"
	"github.com/LOKE/pkg/lokerpc/v2/rpcv1"
	jtd "github.com/jsontypedef/json-typedef-go"
	"google.golang.org/protobuf/proto"
)

// MarshalJSON preserves explicit empty property maps in legacy-compatible metadata.
func (meta Meta) MarshalJSON() ([]byte, error) {
	if !meta.derived {
		return json.Marshal(meta.Meta)
	}
	type endpointJSON struct {
		v1.EndpointMeta
		RequestTypeDef  *schemaJSON `json:"requestTypeDef,omitempty"`
		ResponseTypeDef *schemaJSON `json:"responseTypeDef,omitempty"`
	}
	var interfaces []endpointJSON
	if meta.Interfaces != nil {
		interfaces = make([]endpointJSON, len(meta.Interfaces))
	}
	for i, endpoint := range meta.Interfaces {
		interfaces[i] = endpointJSON{
			EndpointMeta:    endpoint,
			RequestTypeDef:  schemaPointerJSON(endpoint.RequestTypeDef),
			ResponseTypeDef: schemaPointerJSON(endpoint.ResponseTypeDef),
		}
	}
	return json.Marshal(struct {
		v1.Meta
		Interfaces  []endpointJSON        `json:"interfaces"`
		Definitions map[string]schemaJSON `json:"definitions,omitempty"`
	}{
		Meta:        meta.Meta,
		Interfaces:  interfaces,
		Definitions: schemaMapJSON(meta.Definitions),
	})
}

func serviceMetaHandler(meta any, descriptor *rpcv1.ServiceDescriptor) (http.Handler, error) {
	jsonHandler, err := metaHandler(meta)
	if err != nil {
		return nil, err
	}
	var data []byte
	if descriptor != nil {
		data, err = (proto.MarshalOptions{Deterministic: true}).Marshal(descriptor)
		if err != nil {
			return nil, err
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Accept")
		if r.Method == http.MethodGet && descriptor != nil && acceptsProto(r.Header.Values("Accept")) {
			w.Header().Set("Content-Type", "application/proto")
			_, _ = w.Write(data)
			return
		}
		jsonHandler.ServeHTTP(w, r)
	}), nil
}

func acceptsProto(headers []string) bool {
	for _, header := range headers {
		for _, value := range strings.Split(header, ",") {
			mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(value))
			if err != nil || mediaType != "application/proto" {
				continue
			}
			if quality, exists := params["q"]; exists {
				q, err := strconv.ParseFloat(quality, 64)
				if err != nil || !(q > 0 && q <= 1) {
					continue
				}
			}
			return true
		}
	}
	return false
}

type schemaJSON struct {
	Definitions          map[string]schemaJSON  `json:"definitions,omitempty"`
	Metadata             map[string]any         `json:"metadata,omitempty"`
	Nullable             bool                   `json:"nullable,omitempty"`
	Ref                  *string                `json:"ref,omitempty"`
	Type                 jtd.Type               `json:"type,omitempty"`
	Enum                 []string               `json:"enum,omitempty"`
	Elements             *schemaJSON            `json:"elements,omitempty"`
	Properties           *map[string]schemaJSON `json:"properties,omitempty"`
	OptionalProperties   *map[string]schemaJSON `json:"optionalProperties,omitempty"`
	AdditionalProperties bool                   `json:"additionalProperties,omitempty"`
	Values               *schemaJSON            `json:"values,omitempty"`
	Discriminator        string                 `json:"discriminator,omitempty"`
	Mapping              map[string]schemaJSON  `json:"mapping,omitempty"`
}

func schemaValueJSON(schema jtd.Schema) schemaJSON {
	var properties *map[string]schemaJSON
	if schema.Properties != nil {
		converted := schemaMapJSON(schema.Properties)
		properties = &converted
	}
	var optionalProperties *map[string]schemaJSON
	if schema.OptionalProperties != nil {
		converted := schemaMapJSON(schema.OptionalProperties)
		optionalProperties = &converted
	}
	return schemaJSON{
		Definitions:          schemaMapJSON(schema.Definitions),
		Metadata:             schema.Metadata,
		Nullable:             schema.Nullable,
		Ref:                  schema.Ref,
		Type:                 schema.Type,
		Enum:                 schema.Enum,
		Elements:             schemaPointerJSON(schema.Elements),
		Properties:           properties,
		OptionalProperties:   optionalProperties,
		AdditionalProperties: schema.AdditionalProperties,
		Values:               schemaPointerJSON(schema.Values),
		Discriminator:        schema.Discriminator,
		Mapping:              schemaMapJSON(schema.Mapping),
	}
}

func schemaPointerJSON(schema *jtd.Schema) *schemaJSON {
	if schema == nil {
		return nil
	}
	converted := schemaValueJSON(*schema)
	return &converted
}

func schemaMapJSON(schemas map[string]jtd.Schema) map[string]schemaJSON {
	if schemas == nil {
		return nil
	}
	converted := make(map[string]schemaJSON, len(schemas))
	for name, schema := range schemas {
		converted[name] = schemaValueJSON(schema)
	}
	return converted
}
