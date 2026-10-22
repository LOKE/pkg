package lokerpc

import (
	"mime"
	"net/http"

	jtd "github.com/jsontypedef/json-typedef-go"
)

// Codec encodes and decodes Go values directly to and from a binary wire
// format, using the JTD schema from the service meta as the IDL.
//
// Schema pointers are stable for the lifetime of a service or negotiated
// client session, so implementations may cache compiled plans keyed on
// (schema, reflect.Type). Unmarshal's v is always a non-nil pointer.
type Codec interface {
	ContentType() string
	Marshal(v any, schema *jtd.Schema, defs map[string]jtd.Schema) ([]byte, error)
	Unmarshal(data []byte, v any, schema *jtd.Schema, defs map[string]jtd.Schema) error
}

// SchemaFree is implemented by codecs whose wire format carries field names
// (like MessagePack), so a schema change cannot corrupt a message. Requests in
// such a codec need no If-Match and never get a 412.
type SchemaFree interface {
	SchemaFree() bool
}

func isSchemaFree(codec Codec) bool {
	schemaFree, ok := codec.(SchemaFree)
	return ok && schemaFree.SchemaFree()
}

// JSON is a replaceable JSON implementation. The wire format stays JSON; this
// only swaps the engine that reads and writes request and response bodies.
type JSON interface {
	Marshal(v any) ([]byte, error)
	Unmarshal(data []byte, v any) error
}

// ServiceOption configures a Service.
type ServiceOption func(*Service)

// ServeCodecs lets the service negotiate the given binary codecs with clients
// that support them. Clients that don't negotiate keep getting plain JSON.
func ServeCodecs(codecs ...Codec) ServiceOption {
	return func(s *Service) {
		s.codecs = codecs
	}
}

// ServeJSON makes the service decode request bodies straight into the endpoint's
// request type and encode responses with j, instead of encoding/json.
func ServeJSON(j JSON) ServiceOption {
	return func(s *Service) {
		s.json = j
	}
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// ClientCodecs enables binary negotiation, listed in order of preference. The
// client falls back to JSON when the server supports none of them.
func ClientCodecs(codecs ...Codec) ClientOption {
	return func(c *Client) {
		c.codecs = codecs
		c.session = &clientSession{}
	}
}

// ClientJSON makes the client encode request bodies and decode responses with j.
func ClientJSON(j JSON) ClientOption {
	return func(c *Client) {
		c.json = j
	}
}

// WithHTTPClient sets the http.Client used for requests.
func WithHTTPClient(client *http.Client) ClientOption {
	return func(c *Client) {
		c.client = client
	}
}

func findCodec(codecs []Codec, contentType string) Codec {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil
	}
	for _, codec := range codecs {
		if codec.ContentType() == mediaType {
			return codec
		}
	}
	return nil
}
