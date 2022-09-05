package codegen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/LOKE/pkg/lokerpc"
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
