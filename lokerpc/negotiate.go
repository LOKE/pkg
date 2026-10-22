package lokerpc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"mime"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	jtd "github.com/jsontypedef/json-typedef-go"
)

const (
	defaultProbeMaxAge    = 5 * time.Minute
	failedProbeMaxAge     = 30 * time.Second
	serviceMetaMaxAgeSecs = 300
)

// serverNegotiation holds what a service needs to serve binary codecs. The
// schemas are a JSON round-trip of the meta so the server reads them exactly
// as clients do (e.g. an empty struct becomes the empty schema on both sides).
type serverNegotiation struct {
	codecs []Codec
	etag   string
	wire   Meta
}

type endpointNegotiation struct {
	codecs         []Codec
	etag           string
	defs           map[string]jtd.Schema
	requestSchema  *jtd.Schema
	responseSchema *jtd.Schema
}

func newServerNegotiation(codecs []Codec, meta *Meta) (*serverNegotiation, error) {
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return nil, err
	}

	n := &serverNegotiation{codecs: codecs}
	err = json.Unmarshal(metaJSON, &n.wire)
	if err != nil {
		return nil, err
	}

	sum := sha256.Sum256(metaJSON)
	n.etag = `"` + hex.EncodeToString(sum[:]) + `"`
	return n, nil
}

func (n *serverNegotiation) forEndpoint(i int) *endpointNegotiation {
	if n == nil {
		return nil
	}
	endpoint := &n.wire.Interfaces[i]
	return &endpointNegotiation{
		codecs:         n.codecs,
		etag:           n.etag,
		defs:           n.wire.Definitions,
		requestSchema:  endpoint.RequestTypeDef,
		responseSchema: endpoint.ResponseTypeDef,
	}
}

// wrapMetaHandler adds OPTIONS to the service meta handler, advertising the
// codecs and the schema ETag clients must echo in If-Match.
func (n *serverNegotiation) wrapMetaHandler(metaHandler http.HandlerFunc, meta *Meta) http.HandlerFunc {
	if n == nil {
		return metaHandler
	}

	acceptPost := []string{"application/json"}
	for _, codec := range n.codecs {
		acceptPost = append(acceptPost, codec.ContentType())
	}

	return func(rw http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodOptions {
			metaHandler(rw, r)
			return
		}

		rw.Header().Set("Allow", "GET, POST, OPTIONS")
		rw.Header().Set("Accept-Post", strings.Join(acceptPost, ", "))
		rw.Header().Set("Cache-Control", "max-age="+strconv.Itoa(serviceMetaMaxAgeSecs))
		rw.Header().Set("ETag", n.etag)

		asGet := r.Clone(r.Context())
		asGet.Method = http.MethodGet
		metaHandler(rw, asGet)
	}
}

// codecsFor picks the codecs named by the request's Content-Type and Accept.
func (n *endpointNegotiation) codecsFor(r *http.Request) (requestCodec, responseCodec Codec) {
	if n == nil {
		return nil, nil
	}

	requestCodec = findCodec(n.codecs, r.Header.Get("Content-Type"))
	if n.responseSchema == nil {
		return requestCodec, nil
	}

	for _, accepted := range strings.Split(r.Header.Get("Accept"), ",") {
		responseCodec = findCodec(n.codecs, accepted)
		if responseCodec != nil {
			break
		}
	}
	return requestCodec, responseCodec
}

// clientSession is the negotiation state behind a Client, shared by its copies.
type clientSession struct {
	mu      sync.Mutex
	current *negotiated
}

// negotiated is the outcome of probing a server. A nil codec means plain JSON.
type negotiated struct {
	codec     Codec
	etag      string
	endpoints map[string]EndpointMeta
	defs      map[string]jtd.Schema
	expires   time.Time
}

func (s *clientSession) negotiate(ctx context.Context, client *http.Client, probeURL string, preferred []Codec) *negotiated {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.current != nil && time.Now().Before(s.current.expires) {
		return s.current
	}

	s.current = probe(ctx, client, probeURL, preferred)
	return s.current
}

func (s *clientSession) invalidate(stale *negotiated) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.current == stale {
		s.current = nil
	}
}

func probe(ctx context.Context, client *http.Client, probeURL string, preferred []Codec) *negotiated {
	req, err := http.NewRequestWithContext(ctx, http.MethodOptions, probeURL, nil)
	if err != nil {
		return &negotiated{expires: time.Now().Add(failedProbeMaxAge)}
	}

	res, err := client.Do(req)
	if err != nil {
		return &negotiated{expires: time.Now().Add(failedProbeMaxAge)}
	}
	defer res.Body.Close()

	result := &negotiated{expires: time.Now().Add(maxAge(res.Header.Get("Cache-Control")))}
	if res.StatusCode != http.StatusOK {
		return result
	}

	var serverTypes []string
	for _, accepted := range strings.Split(res.Header.Get("Accept-Post"), ",") {
		mediaType, _, err := mime.ParseMediaType(accepted)
		if err == nil {
			serverTypes = append(serverTypes, mediaType)
		}
	}

	sharedIdx := slices.IndexFunc(preferred, func(codec Codec) bool {
		return slices.Contains(serverTypes, codec.ContentType())
	})
	if sharedIdx < 0 {
		return result
	}

	etag := res.Header.Get("ETag")
	if etag == "" {
		return result
	}

	var meta Meta
	err = json.NewDecoder(res.Body).Decode(&meta)
	if err != nil {
		return result
	}

	endpoints := make(map[string]EndpointMeta, len(meta.Interfaces))
	for _, endpoint := range meta.Interfaces {
		endpoints[endpoint.MethodName] = endpoint
	}

	result.codec = preferred[sharedIdx]
	result.etag = etag
	result.endpoints = endpoints
	result.defs = meta.Definitions
	return result
}

func maxAge(cacheControl string) time.Duration {
	for _, directive := range strings.Split(cacheControl, ",") {
		value, ok := strings.CutPrefix(strings.TrimSpace(directive), "max-age=")
		if !ok {
			continue
		}
		seconds, err := strconv.Atoi(value)
		if err == nil && seconds >= 0 {
			return time.Duration(seconds) * time.Second
		}
	}
	return defaultProbeMaxAge
}

// endpoint returns the method's schemas, if a codec was negotiated and the
// server knows the method.
func (n *negotiated) endpoint(method string) (EndpointMeta, bool) {
	if n == nil || n.codec == nil {
		return EndpointMeta{}, false
	}
	endpoint, ok := n.endpoints[method]
	return endpoint, ok
}
