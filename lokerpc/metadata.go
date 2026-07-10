package lokerpc

import (
	"fmt"
	"reflect"
	"sync"
)

const (
	descriptionMetadataKey = "description"
	enumNamesMetadataKey   = "enumNames"
	enumTypeMetadataKey    = "enumType"
)

// EnumValue describes one named string constant belonging to a Go type.
type EnumValue struct {
	Name  string
	Value string
}

// TypeMetadata contains source information that Go reflection does not retain.
type TypeMetadata struct {
	Description string
	Fields      map[string]string
	Enum        []EnumValue
}

// ServiceMetadata contains documentation for a service interface and its methods.
type ServiceMetadata struct {
	Description string
	Methods     map[string]string
}

// PackageMetadata is the compact source metadata emitted by lokerpcgen.
type PackageMetadata struct {
	Types    map[string]TypeMetadata
	Services map[string]ServiceMetadata
}

var generatedMetadata = struct {
	sync.RWMutex
	packages map[string]PackageMetadata
}{
	packages: make(map[string]PackageMetadata),
}

// RegisterPackageMetadata registers metadata generated for one Go package.
// Applications should run lokerpcgen instead of calling this directly.
func RegisterPackageMetadata(packagePath string, metadata PackageMetadata) {
	generatedMetadata.Lock()
	defer generatedMetadata.Unlock()

	if _, exists := generatedMetadata.packages[packagePath]; exists {
		panic("lokerpc: metadata already registered for " + packagePath)
	}
	generatedMetadata.packages[packagePath] = metadata
}

func metadataForType(t reflect.Type) (TypeMetadata, bool) {
	generatedMetadata.RLock()
	defer generatedMetadata.RUnlock()

	metadata, ok := generatedMetadata.packages[t.PkgPath()]
	if !ok {
		return TypeMetadata{}, false
	}
	typeMetadata, ok := metadata.Types[t.Name()]
	return typeMetadata, ok
}

// WithGeneratedDocs fills service and endpoint help from a generated Go service
// interface. Existing help is retained for undocumented methods.
func WithGeneratedDocs[ServiceInterface any]() ServiceOption {
	t := reflect.TypeFor[ServiceInterface]()
	if t == nil || t.Kind() != reflect.Interface {
		panic("lokerpc: WithGeneratedDocs requires an interface type")
	}

	return func(service *Service) {
		generatedMetadata.RLock()
		packageMetadata, packageExists := generatedMetadata.packages[t.PkgPath()]
		serviceMetadata, serviceExists := packageMetadata.Services[t.Name()]
		generatedMetadata.RUnlock()

		if !packageExists || !serviceExists {
			panic(fmt.Sprintf(
				"lokerpc: no generated docs for %s; run go generate in its package",
				t.String(),
			))
		}

		if serviceMetadata.Description != "" {
			service.Help = serviceMetadata.Description
		}
		for rpcName, endpoint := range service.endpointCodecs {
			goName := exportedName(rpcName)
			if help := serviceMetadata.Methods[goName]; help != "" {
				endpoint.Help = help
				service.endpointCodecs[rpcName] = endpoint
			}
		}
	}
}

func exportedName(name string) string {
	if name == "" || name[0] < 'a' || name[0] > 'z' {
		return name
	}
	return string(name[0]-('a'-'A')) + name[1:]
}
