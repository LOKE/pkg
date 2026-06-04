package lokerpc

import "sync"

// TypeDocEntry holds pre-extracted documentation for a Go struct type.
type TypeDocEntry struct {
	Doc    string            // struct-level godoc text
	Fields map[string]string // JSON-key or Go field name → comment text
}

var (
	regDocsMu sync.RWMutex
	regDocs   = map[string]map[string]TypeDocEntry{} // pkgImportPath → typeName → entry
)

// RegisterTypeDocs registers pre-extracted documentation for Go struct types in the
// given package. Call this from init() in generated rpc_docs_gen.go files so that
// documentation is available in production binaries without requiring source files on disk.
func RegisterTypeDocs(pkgPath string, entries map[string]TypeDocEntry) {
	regDocsMu.Lock()
	defer regDocsMu.Unlock()
	regDocs[pkgPath] = entries
}

func lookupRegisteredDocs(pkgPath, typeName string) (TypeDocEntry, bool) {
	regDocsMu.RLock()
	defer regDocsMu.RUnlock()
	if pkg, ok := regDocs[pkgPath]; ok {
		entry, ok := pkg[typeName]
		return entry, ok
	}
	return TypeDocEntry{}, false
}
