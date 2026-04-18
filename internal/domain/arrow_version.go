package domain

// ArrowVersion is the compiled, runtime-ready representation of a single installed
// arrow version. It embeds ArrowManifest and adds install-tracking metadata.
// ArrowManifest remains the vault/manifold transfer type and is defined in arrow.go.
type ArrowVersion struct {
	ArrowManifest
	InstalledRef string `json:"installed_ref"`
}
