package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGeneratedFilesAreCurrent(t *testing.T) {
	files, err := render(".")
	if err != nil {
		t.Fatal(err)
	}

	for name, want := range files {
		t.Run(name, func(t *testing.T) {
			got, err := os.ReadFile(name)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("%s is stale, run go generate ./...", name)
			}
		})
	}
}

func TestMsgpFilesAreCurrent(t *testing.T) {
	tests := []struct {
		source    string
		generated string
	}{
		{"../fixtures/fixtures.go", "../fixtures/msgp_gen.go"},
		{"../fixtures/menuclient/menuclient.go", "../fixtures/menuclient/msgp_gen.go"},
	}

	for _, tt := range tests {
		t.Run(tt.generated, func(t *testing.T) {
			regenerated := filepath.Join(t.TempDir(), "msgp_gen.go")
			command := exec.Command("go", "run", "github.com/tinylib/msgp", "-file", tt.source, "-o", regenerated, "-io=false", "-tests=false")
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("msgp: %v\n%s", err, output)
			}

			want, err := os.ReadFile(regenerated)
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(tt.generated)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("%s is stale, run go generate ./...", tt.generated)
			}
		})
	}
}
