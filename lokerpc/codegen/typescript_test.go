package codegen

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LOKE/pkg/lokerpc"
	jtd "github.com/jsontypedef/json-typedef-go"
)

func TestGenTypescriptClient(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("testdata", "*.json"))
	if err != nil {
		t.Fatal(err)
	}

	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			var meta lokerpc.Meta

			f, err := os.Open(p)
			if err != nil {
				t.Fatal(err)
			}

			if err := json.NewDecoder(f).Decode(&meta); err != nil {
				t.Fatal(err)
			}

			dst, err := os.Create(p + ".ts")
			if err != nil {
				t.Fatal(err)
			}

			if err := GenTypescriptClient(dst, meta); err != nil {
				t.Fatal(err)

			}

		})
	}
}

func TestGenTypescriptClient_DiscriminatorRequestProducesUnionType(t *testing.T) {
	meta := lokerpc.Meta{
		ServiceName: "email",
		Help:        "Email service",
		Interfaces: []lokerpc.EndpointMeta{
			{
				MethodName: "send",
				Help:       "Send email",
				RequestTypeDef: &jtd.Schema{
					Discriminator: "template",
					Mapping: map[string]jtd.Schema{
						"loke_manager_magic_link": {
							Properties: map[string]jtd.Schema{
								"template": {Enum: []string{"loke_manager_magic_link"}},
								"to":       {Type: jtd.TypeString},
							},
						},
						"loke_manager_otp": {
							Properties: map[string]jtd.Schema{
								"template": {Enum: []string{"loke_manager_otp"}},
								"to":       {Type: jtd.TypeString},
							},
						},
					},
				},
			},
		},
	}

	var out bytes.Buffer
	if err := GenTypescriptClient(&out, meta); err != nil {
		t.Fatal(err)
	}

	got := out.String()

	for _, want := range []string{
		"export type SendRequest =",
		"template: \"loke_manager_magic_link\";",
		"template: \"loke_manager_otp\";",
		"send(ctx: Context, req: SendRequest): Promise<any>",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("generated output missing %q\n--- output ---\n%s", want, got)
		}
	}
}
