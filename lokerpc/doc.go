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
//		lokerpc.GeneratedDocs[Service](),
//		endpoints,
//	)
//
// The generator follows local request and response types reachable from the
// selected interface. Shared DTO packages can register their own roots with
// repeatable -type flags. It preserves type and field doc comments as JTD
// description metadata, and exposes named string constants as JTD enums.
// Generated services use MakeGeneratedStandardEndpointCodec or
// MakeGeneratedVoidEndpointCodec so no empty help placeholders are needed.
// Existing services may continue passing manual help strings to NewService and
// the original endpoint codec constructors.
package lokerpc
