package lokerpc_test

import (
	"context"
	"errors"
	"math"
	"net/http"
	"testing"

	"connectrpc.com/connect"
	"github.com/LOKE/pkg/lokerpc/v2/internal/testpb"
	"github.com/LOKE/pkg/lokerpc/v2/rpcv1"
)

func TestBridgeLegacyJSONStringifyBytes(t *testing.T) {
	for _, test := range []struct {
		name    string
		message string
		detail  *rpcv1.Error
		want    string
	}{
		{
			name:    "raw Unicode and HTML without rewriting literal escapes",
			message: "<>&\u2028\u2029😀",
			detail: &rpcv1.Error{Code: "unicode", Meta: fixtureMeta(t, map[string]any{
				"text":     "<>&\u2028\u2029😀",
				"literal":  `\u2028\u2029\\u2028`,
				"controls": "\x00\b\f\n\r\t\"\\",
			})},
			want: `{"message":"<>&` + "\u2028\u2029😀" + `","instance":"","code":"unicode","type":"unicode","controls":"\u0000\b\f\n\r\t\"\\","literal":"\\u2028\\u2029\\\\u2028","text":"<>&` + "\u2028\u2029😀" + `"}`,
		},
		{
			name:    "integer indices precede fixed fields and nested names",
			message: "failure",
			detail: &rpcv1.Error{Code: "ordered", Meta: fixtureMeta(t, map[string]any{
				"10": "ten", "2": "two", "0": "zero", "4294967294": "last index",
				"4294967295": "not an index", "01": "leading zero", "-0": "negative zero",
				"nested": map[string]any{"10": true, "2": false, "z": nil, "a": []any{map[string]any{"10": 10, "2": 2}}},
				"\ue000": "BMP", "😀": "supplementary",
			})},
			want: `{"0":"zero","2":"two","10":"ten","4294967294":"last index","message":"failure","instance":"","code":"ordered","type":"ordered","-0":"negative zero","01":"leading zero","4294967295":"not an index","nested":{"2":false,"10":true,"a":[{"2":2,"10":10}],"z":null},"😀":"supplementary","` + "\ue000" + `":"BMP"}`,
		},
		{
			name:    "nonfinite proto numbers become null recursively",
			message: "failure",
			detail: &rpcv1.Error{Code: "numbers", Meta: fixtureMeta(t, map[string]any{
				"nan": math.NaN(), "positive": math.Inf(1), "negative": math.Inf(-1),
				"nested": map[string]any{"values": []any{math.NaN(), math.Inf(1), math.Inf(-1), "NaN", "Infinity"}},
			})},
			want: `{"message":"failure","instance":"","code":"numbers","type":"numbers","nan":null,"negative":null,"nested":{"values":[null,null,null,"NaN","Infinity"]},"positive":null}`,
		},
		{
			name:    "ECMAScript number notation",
			message: "failure",
			detail: &rpcv1.Error{Code: "numbers", Meta: fixtureMeta(t, map[string]any{
				"values": []any{math.Copysign(0, -1), 1e-7, 1e-6, 1e20, 1e21, -1.25e-9},
			})},
			want: `{"message":"failure","instance":"","code":"numbers","type":"numbers","values":[0,1e-7,0.000001,100000000000000000000,1e+21,-1.25e-9]}`,
		},
		{
			name:    "attached metadata cannot override identity",
			message: "failure",
			detail: &rpcv1.Error{Code: "original", Instance: "original", Meta: fixtureMeta(t, map[string]any{
				"message": "override", "instance": nil, "code": "meta", "type": "meta-type",
				"expose": false, "namespace": "meta-namespace", "help": "meta-help", "name": "meta-name", "stack": "meta-stack",
				"nested": map[string]any{"message": "nested message", "code": "nested code"},
				"other":  "retained",
			})},
			want: `{"message":"failure","instance":"original","code":"original","type":"original","nested":{"code":"nested code","message":"nested message"},"other":"retained"}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			detail, err := connect.NewErrorDetail(test.detail)
			if err != nil {
				t.Fatal(err)
			}
			failure := connect.NewError(connect.CodeInvalidArgument, errors.New(test.message))
			failure.AddDetail(detail)
			mux := mountFixture(t, fixtureService("fixture", fixtureHandler{echo: func(context.Context, *connect.Request[testpb.Scalars]) (*connect.Response[testpb.Scalars], error) {
				return nil, failure
			}}))
			response := serve(t, mux, http.MethodPost, "/rpc/fixture/echo", "{}")
			if response.Code != http.StatusBadRequest {
				t.Errorf("bridge status = %d, want 400", response.Code)
			}
			if got := response.Body.String(); got != test.want {
				t.Errorf("bridge body = %q, want %q", got, test.want)
			}
		})
	}
}
