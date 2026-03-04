package lokerpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/LOKE/pkg/requestid"
)

func NewClient(baseURL string) Client {
	return newClientWithClient(baseURL, http.DefaultClient)
}

func newClientWithClient(baseURL string, client *http.Client) Client {
	return Client{bURL: baseURL, client: client}
}

// NOTE: Maybe this should be exported, leaving it for now -- Dom
type rpcClientError struct {
	Message   string
	Instance  string
	Expose    bool
	Code      string
	Namespace string
	Type      string
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

func (c Client) DoRequest(ctx context.Context, method string, args, result any) error {
	b := new(bytes.Buffer)
	if err := json.NewEncoder(b).Encode(args); err != nil {
		return err
	}

	url := c.bURL + method
	req, err := http.NewRequest("POST", url, b)
	if err != nil {
		return err
	}

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
