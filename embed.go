// Package quivercore exists only to embed the repo-root ARROW.md: go:embed cannot
// reach a file above its own package directory.
package quivercore

import _ "embed"

//go:embed ARROW.md
var selfManifest []byte

// SelfManifest returns quiver.core's own arrow manifest, embedded at build time.
func SelfManifest() []byte {
	return selfManifest
}
