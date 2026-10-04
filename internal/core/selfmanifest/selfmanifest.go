// Package selfmanifest exposes quiver.core's own ARROW.md.
package selfmanifest

import quivercore "github.com/rabbytesoftware/quiver.core"

// Raw returns quiver.core's own arrow manifest, embedded into the binary at build time.
func Raw() []byte {
	return quivercore.SelfManifest()
}
