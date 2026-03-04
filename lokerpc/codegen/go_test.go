package codegen

import (
	"bytes"
	"encoding/json"
	"go/format"
	"os"
	"path/filepath"
	"testing"

	"github.com/LOKE/pkg/lokerpc"
)

func TestGenGoClient(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("testdata", "*.json"))
	if err != nil {
		t.Fatal(err)
	}

	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			var meta lokerpc.Meta

			// TODO: go gen doesn't yet support discriminators
			if t.Name() == "TestGenGoClient/testdata/discriminator.json" {
				return
			}

			// 😠 don't like that "union" meta tag got let in as a supported
			// feature. It's really not portable, and there is no way for
			// statically typed languages to support it
			if t.Name() == "TestGenGoClient/testdata/union-metadata.json" {
				return
			}

			f, err := os.Open(p)
			if err != nil {
				t.Fatal(err)
			}

			if err := json.NewDecoder(f).Decode(&meta); err != nil {
				t.Fatal(err)
			}

			var buf bytes.Buffer

			if err := GenGoClient(&buf, meta); err != nil {
				t.Fatal(err)
			}

			formatted, err := format.Source(buf.Bytes())

			err = os.WriteFile(p+".go", formatted, 0644)
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
