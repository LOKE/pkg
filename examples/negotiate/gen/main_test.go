package main

import (
	"bytes"
	"os"
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
