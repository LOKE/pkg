package main

import (
	"bytes"
	"io"
	"net/http"
	"sync"
)

// Exchange is one HTTP round trip as seen on the wire.
type Exchange struct {
	Method       string
	Path         string
	Status       int
	RequestType  string
	ResponseType string
	IfMatch      string
	ETag         string
	RequestBody  []byte
	ResponseBody []byte
}

// Recorder is an http.RoundTripper that remembers every exchange. It only
// observes; bodies pass through untouched.
type Recorder struct {
	Base http.RoundTripper

	mu        sync.Mutex
	exchanges []Exchange
}

func (r *Recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	exchange := Exchange{
		Method:      req.Method,
		Path:        req.URL.Path,
		RequestType: req.Header.Get("Content-Type"),
		IfMatch:     req.Header.Get("If-Match"),
	}

	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, err
		}
		exchange.RequestBody, err = io.ReadAll(body)
		if err != nil {
			return nil, err
		}
	}

	base := r.Base
	if base == nil {
		base = http.DefaultTransport
	}
	res, err := base.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	exchange.ResponseBody, err = io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		return nil, err
	}
	res.Body = io.NopCloser(bytes.NewReader(exchange.ResponseBody))

	exchange.Status = res.StatusCode
	exchange.ResponseType = res.Header.Get("Content-Type")
	exchange.ETag = res.Header.Get("ETag")

	r.mu.Lock()
	r.exchanges = append(r.exchanges, exchange)
	r.mu.Unlock()
	return res, nil
}

// Take returns the exchanges recorded since the last call.
func (r *Recorder) Take() []Exchange {
	r.mu.Lock()
	defer r.mu.Unlock()

	taken := r.exchanges
	r.exchanges = nil
	return taken
}
