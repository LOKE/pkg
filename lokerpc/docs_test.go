package lokerpc

import (
	"go/ast"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParsePkgDocs(t *testing.T) {
	testdataDir := filepath.Join("testdata", "doctest")
	docs, err := parsePkgDocs(testdataDir)
	if err != nil {
		t.Fatalf("parsePkgDocs failed: %v", err)
	}

	if docs == nil {
		t.Fatal("docs is nil")
	}

	if len(docs.types) == 0 {
		t.Fatal("no types extracted")
	}

	// Check Person type
	personDoc, ok := docs.types["Person"]
	if !ok {
		t.Fatalf("Person type not found in docs")
	}

	if !strings.Contains(personDoc.doc, "Person represents a person") {
		t.Errorf("Person doc unexpected: %q", personDoc.doc)
	}

	// Check Person fields
	if nameDoc, ok := personDoc.fields["name"]; !ok || !strings.Contains(nameDoc, "full name") {
		t.Errorf("Person.name field doc unexpected: %q", nameDoc)
	}

	if ageDoc, ok := personDoc.fields["age"]; !ok || !strings.Contains(ageDoc, "age in years") {
		t.Errorf("Person.age field doc unexpected: %q", ageDoc)
	}

	if emailDoc, ok := personDoc.fields["email"]; !ok || !strings.Contains(emailDoc, "optional") {
		t.Errorf("Person.email field doc unexpected: %q", emailDoc)
	}

	// Check Request type
	requestDoc, ok := docs.types["Request"]
	if !ok {
		t.Fatalf("Request type not found in docs")
	}

	if !strings.Contains(requestDoc.doc, "test request type") {
		t.Errorf("Request doc unexpected: %q", requestDoc.doc)
	}

	// Check UndocumentedType - should exist with its doc
	undocDoc, ok := docs.types["UndocumentedType"]
	if !ok {
		t.Fatalf("UndocumentedType not found in docs")
	}

	if !strings.Contains(undocDoc.doc, "no documentation") {
		t.Errorf("UndocumentedType should have its doc, got: %q", undocDoc.doc)
	}
}

func TestFieldJSONKey(t *testing.T) {
	tests := []struct {
		name     string
		tagValue string
		want     string
	}{
		{
			name:     "json tag with name",
			tagValue: "`json:\"myfield\"`",
			want:     "myfield",
		},
		{
			name:     "json tag with omitempty",
			tagValue: "`json:\"myfield,omitempty\"`",
			want:     "myfield",
		},
		{
			name:     "json tag with dash (falls through to field name)",
			tagValue: "`json:\"-\"`",
			want:     "TestField", // dash is excluded, falls back to field name
		},
		{
			name:     "no tag uses field name",
			tagValue: "",
			want:     "TestField",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock field
			field := &ast.Field{
				Names: []*ast.Ident{{Name: "TestField"}},
			}
			if tt.tagValue != "" {
				field.Tag = &ast.BasicLit{Value: tt.tagValue}
			}

			got := fieldJSONKey(field)
			if got != tt.want {
				t.Errorf("fieldJSONKey = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoadPkgDocsCache(t *testing.T) {
	testdataDir := filepath.Join("testdata", "doctest")

	// First call
	docs1, err1 := loadPkgDocs(testdataDir)
	if err1 != nil {
		t.Fatalf("first loadPkgDocs failed: %v", err1)
	}

	// Second call should return cached result
	docs2, err2 := loadPkgDocs(testdataDir)
	if err2 != nil {
		t.Fatalf("second loadPkgDocs failed: %v", err2)
	}

	// Should be the same object (cached)
	if docs1 != docs2 {
		t.Error("docs should be cached (same object)")
	}

	// But content should be identical
	if len(docs1.types) != len(docs2.types) {
		t.Errorf("cached docs have different number of types: %d vs %d", len(docs1.types), len(docs2.types))
	}
}

func TestPkgSourceDirExternalPackage(t *testing.T) {
	// External packages should be rejected
	_, err := pkgSourceDir("encoding/json")
	if err == nil {
		t.Error("external package should be rejected")
	}
}

func TestEnrichWithDocsIntegration(t *testing.T) {
	// This is a basic integration test that enrichment is called
	// In a real scenario, we would test with actual types from the testdata package
	// but reflection type resolution requires the package to be compiled and imported

	// Create a mock logger
	logger := newMockLogger()

	// Empty defs should return empty map
	defs := map[reflect.Type]*NamedSchema{}
	docs := EnrichWithDocs(defs, logger)

	if len(docs) > 0 {
		t.Errorf("expected no docs for empty defs, got %d", len(docs))
	}
}

type mockLogger struct{}

func newMockLogger() *mockLogger {
	return &mockLogger{}
}

func (m *mockLogger) Log(keyvals ...interface{}) error {
	return nil
}
