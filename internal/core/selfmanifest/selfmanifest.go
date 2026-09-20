// Package selfmanifest exposes quiver.core's own embedded ARROW.md.
package selfmanifest

import (
	quivercore "github.com/rabbytesoftware/quiver.core"
)

// Raw returns quiver.core's own arrow manifest, embedded into the binary at
// build time so it always matches exactly what's running — no network
// fetch, no drift between a release and the repo state it was cut from.
//
// The //go:embed directive itself lives in the module-root package (see
// arrow_manifest_embed.go) because go:embed cannot reach outside the
// directory of the file that declares it; this package only re-exports
// those bytes so the rest of internal/ has a normal internal/core/ entry
// point to depend on.
func Raw() []byte {
	return quivercore.ArrowManifest
}
