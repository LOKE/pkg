// Package lokerpc implements typed JSON RPC services and client metadata.
//
// Go source documentation and constants can be added to the exposed metadata
// without embedding source code. Add a generator directive to the service
// package:
//
//	//go:generate go run github.com/LOKE/pkg/lokerpc/cmd/lokerpcgen -service Service
//
// Then opt the service into its generated documentation:
//
//	lokerpc.NewService(
//		"payments",
//		"fallback help",
//		endpoints,
//		lokerpc.WithGeneratedDocs[Service](),
//	)
//
// The generator follows local request and response types reachable from the
// selected interface. Shared DTO packages can register their own roots with
// repeatable -type flags. It preserves type and field doc comments as JTD
// description metadata, and exposes named string constants as JTD enums.
// Existing help text remains the fallback for undocumented interface methods.
package lokerpc
