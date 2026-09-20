// Package quivercore holds nothing but ARROW.md's embed directive.
//
// The embed directive can only reach files within the directory tree rooted
// at the source file that declares it: any ".." element in its pattern is a
// hard compile error ("invalid pattern syntax"), regardless of depth, so no
// file under internal/ can embed a repository-root file directly. ARROW.md
// lives at the repository root by convention — it is quiver.core's own
// arrow manifest, resolved the same way any other arrow's root-level
// manifest is — so the one file allowed to embed it must live here too.
// internal/core/selfmanifest re-exports these bytes; nothing else should
// import this package.
package quivercore

import _ "embed"

//go:embed ARROW.md
var ArrowManifest []byte
