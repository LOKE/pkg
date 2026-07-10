package lokerpc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/go-kit/log"
	jtd "github.com/jsontypedef/json-typedef-go"
)

type generatedDocsStatus string

type generatedDocsRequest struct {
	Status generatedDocsStatus `json:"status"`
	Note   string              `json:"note"`
}

type generatedDocsService interface {
	Lookup(context.Context, generatedDocsRequest) (generatedDocsRequest, error)
	Undocumented(context.Context, generatedDocsRequest) (generatedDocsRequest, error)
}

func registerTestMetadata() {
	packagePath := reflect.TypeFor[generatedDocsRequest]().PkgPath()
	generatedMetadata.Lock()
	delete(generatedMetadata.packages, packagePath)
	generatedMetadata.Unlock()
	RegisterPackageMetadata(packagePath, PackageMetadata{
		Types: map[string]TypeMetadata{
			"generatedDocsRequest": {
				Description: "generatedDocsRequest is used to find a record.",
				Fields: map[string]string{
					"Status": "Status limits the result.",
				},
			},
			"generatedDocsStatus": {
				Description: "generatedDocsStatus is the record state.",
				Enum: []EnumValue{
					{Name: "generatedDocsStatusOpen", Value: "open"},
					{Name: "generatedDocsStatusClosed", Value: "closed"},
				},
			},
		},
		Services: map[string]ServiceMetadata{
			"generatedDocsService": {
				Description: "generatedDocsService finds records.",
				Methods: map[string]string{
					"Lookup": "Lookup finds one record.",
				},
			},
		},
	})
}

func TestGeneratedMetadataDecoratesServiceAndSchema(t *testing.T) {
	registerTestMetadata()

	service := NewService("records", GeneratedDocs[generatedDocsService](), EndpointCodecMap{
		"lookup": MakeGeneratedStandardEndpointCodec(
			func(_ context.Context, request generatedDocsRequest) (generatedDocsRequest, error) {
				return request, nil
			},
		),
		"undocumented": MakeStandardEndpointCodec(
			func(_ context.Context, request generatedDocsRequest) (generatedDocsRequest, error) {
				return request, nil
			},
			"manual fallback",
		),
	})

	if service.Help != "generatedDocsService finds records." {
		t.Fatalf("service help = %q", service.Help)
	}
	if got := service.endpointCodecs["lookup"].Help; got != "Lookup finds one record." {
		t.Fatalf("lookup help = %q", got)
	}
	if got := service.endpointCodecs["undocumented"].Help; got != "manual fallback" {
		t.Fatalf("undocumented help = %q", got)
	}

	definitions := map[reflect.Type]*NamedSchema{}
	requestSchema := TypeSchema(reflect.TypeFor[generatedDocsRequest](), definitions)
	if requestSchema.Ref == nil || *requestSchema.Ref != "generatedDocsRequest" {
		t.Fatalf("request schema = %#v", requestSchema)
	}
	gotDefinitions := TypeDefs(definitions)

	requestDefinition := gotDefinitions["generatedDocsRequest"]
	if got := requestDefinition.Metadata["description"]; got != "generatedDocsRequest is used to find a record." {
		t.Fatalf("request description = %#v", got)
	}
	statusProperty := requestDefinition.Properties["status"]
	if got := statusProperty.Metadata["description"]; got != "Status limits the result." {
		t.Fatalf("field description = %#v", got)
	}

	statusDefinition := gotDefinitions["generatedDocsStatus"]
	wantEnum := []string{"open", "closed"}
	if !reflect.DeepEqual(statusDefinition.Enum, wantEnum) {
		t.Fatalf("enum = %#v, want %#v", statusDefinition.Enum, wantEnum)
	}
	if statusDefinition.Form() != jtd.FormEnum {
		t.Fatalf("enum form = %v", statusDefinition.Form())
	}
	if got := statusDefinition.Metadata["description"]; got != "generatedDocsStatus is the record state." {
		t.Fatalf("enum description = %#v", got)
	}

	mux := http.NewServeMux()
	MountHandlers(log.NewNopLogger(), mux, service)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest("GET", "/rpc/records", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("metadata status = %d", response.Code)
	}
	var exposed Meta
	if err := json.NewDecoder(response.Body).Decode(&exposed); err != nil {
		t.Fatal(err)
	}
	if exposed.Help != "generatedDocsService finds records." {
		t.Fatalf("exposed service help = %q", exposed.Help)
	}
	if got := exposed.Definitions["generatedDocsStatus"].Enum; !reflect.DeepEqual(got, wantEnum) {
		t.Fatalf("exposed enum = %#v", got)
	}
}

func TestGeneratedDocsRequiresGeneratedMetadata(t *testing.T) {
	type missingService interface {
		Missing(context.Context, generatedDocsRequest) (generatedDocsRequest, error)
	}

	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("GeneratedDocs did not panic")
		}
	}()
	NewService("missing", GeneratedDocs[missingService](), EndpointCodecMap{})
}

func TestGeneratedServiceDocsRejectsZeroValue(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("zero-value GeneratedServiceDocs did not panic")
		}
	}()
	NewService("invalid", GeneratedServiceDocs{}, EndpointCodecMap{})
}
