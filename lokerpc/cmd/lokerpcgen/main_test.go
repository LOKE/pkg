package main

import (
	"fmt"
	"go/ast"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildMetadataAndRender(t *testing.T) {
	directory := t.TempDir()
	sourceText := `package example

import "context"

// Service reads widgets.
type Service interface {
	CommonService

	// Lookup returns a widget.
	Lookup(context.Context, LookupRequest) (*Widget, error)
}

type CommonService interface {
	// Health reports service health.
	Health(context.Context, LookupRequest) (*Widget, error)
}

// State is a widget state.
type State string

const (
	StateOpen State = "open"
	StateClosed = State("closed")
	StateExtended = (StateClosed) + "-extended"
)

// LookupRequest selects a widget.
type LookupRequest struct {
	// State filters widgets.
	State State ` + "`json:\"state\"`" + `
	Label string ` + "`json:\"label\"`" + ` // Label is free-form.
}

type Widget struct {
	State State ` + "`json:\"state\"`" + `
}

type InternalState string

const InternalStateHidden InternalState = "hidden"
`
	if err := os.WriteFile(filepath.Join(directory, "example.go"), []byte(sourceText), 0o644); err != nil {
		t.Fatal(err)
	}

	info := packageInfo{
		Dir:        directory,
		ImportPath: "example.com/project/example",
		Name:       "example",
		GoFiles:    []string{"example.go"},
	}
	source, err := parsePackage(info, defaultOutput)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := buildMetadata(source, []string{"Service"}, nil)
	if err != nil {
		t.Fatal(err)
	}

	if _, exists := metadata.types["InternalState"]; exists {
		t.Fatal("unreachable type was included")
	}
	request := metadata.types["LookupRequest"]
	if request.fields["State"] != "State filters widgets." {
		t.Fatalf("leading field doc = %q", request.fields["State"])
	}
	if request.fields["Label"] != "Label is free-form." {
		t.Fatalf("trailing field doc = %q", request.fields["Label"])
	}
	if got := metadata.types["State"].enum; len(got) != 3 || got[2].value != "closed-extended" {
		t.Fatalf("enum = %#v", got)
	}
	if metadata.services["Service"].methods["Health"] != "Health reports service health." {
		t.Fatalf("embedded method docs = %#v", metadata.services["Service"].methods)
	}

	typeOnly, err := buildMetadata(source, nil, []string{"State"})
	if err != nil {
		t.Fatal(err)
	}
	if len(typeOnly.types["State"].enum) != 3 {
		t.Fatalf("type-only enum = %#v", typeOnly.types["State"].enum)
	}

	generated, err := render(metadata)
	if err != nil {
		t.Fatal(err)
	}
	output := string(generated)
	for _, want := range []string{
		`RegisterPackageMetadata("example.com/project/example"`,
		`Description: "Service reads widgets."`,
		`"Lookup": "Lookup returns a widget."`,
		`{Name: "StateExtended", Value: "closed-extended"}`,
	} {
		if !strings.Contains(output, want) {
			t.Errorf("generated output does not contain %q:\n%s", want, output)
		}
	}

	if err := os.WriteFile(filepath.Join(directory, defaultOutput), generated, 0o644); err != nil {
		t.Fatal(err)
	}
	repositoryRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	goMod := fmt.Sprintf(`module example.com/project/example

go 1.26.0

require github.com/LOKE/pkg v0.0.0

replace github.com/LOKE/pkg => %s
`, repositoryRoot)
	if err := os.WriteFile(filepath.Join(directory, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}
	integrationTest := `package example

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LOKE/pkg/lokerpc"
	"github.com/go-kit/log"
)

func TestGeneratedRegistration(t *testing.T) {
	service := lokerpc.NewService("widgets", lokerpc.GeneratedDocs[Service](), lokerpc.EndpointCodecMap{
		"lookup": lokerpc.MakeGeneratedStandardEndpointCodec(
			func(_ context.Context, request LookupRequest) (*Widget, error) {
				return &Widget{State: request.State}, nil
			},
		),
	})
	mux := http.NewServeMux()
	lokerpc.MountHandlers(log.NewNopLogger(), mux, service)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest("GET", "/rpc/widgets", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	if body := response.Body.String(); !strings.Contains(body, ` + "`\"enum\":[\"open\",\"closed\",\"closed-extended\"]`" + `) {
		t.Fatalf("metadata = %s", body)
	}
}
`
	if err := os.WriteFile(filepath.Join(directory, "metadata_integration_test.go"), []byte(integrationTest), 0o644); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("go", "test", "-mod=mod", "./...")
	command.Dir = directory
	command.Env = append(os.Environ(), "GOWORK=off")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("generated package test failed: %v\n%s", err, output)
	}
}

func TestBuildMetadataRejectsNonInterface(t *testing.T) {
	source := sourcePackage{
		types: map[string]typeDeclaration{
			"Service": {spec: &ast.TypeSpec{Name: ast.NewIdent("Service"), Type: ast.NewIdent("string")}},
		},
	}
	_, err := buildMetadata(source, []string{"Service"}, nil)
	if err == nil || !strings.Contains(err.Error(), "not an interface") {
		t.Fatalf("error = %v", err)
	}
}
