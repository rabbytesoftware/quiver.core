// Package selfmanifest exposes quiver.core's own embedded ARROW.md.
package selfmanifest

import _ "embed"

//go:embed ARROW.md
var arrowManifest []byte

// Raw returns quiver.core's own arrow manifest, embedded into the binary at build time.
func Raw() []byte {
	return arrowManifest
}
