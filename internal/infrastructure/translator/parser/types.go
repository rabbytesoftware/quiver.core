package parser

// Parser defines the interface for parsing manifest strings and YAML data
type Parser interface {
	// Parse parses a manifest string in the format "schema@version"
	Parse(manifestStr string) (*ManifestInfo, error)

	// ParseFromYAML extracts and parses the manifest field from YAML data
	ParseFromYAML(yamlData []byte) (*ManifestInfo, error)
}

// ManifestInfo contains the parsed information from a manifest string
type ManifestInfo struct {
	// SchemaType is the type of schema (arrow or quiver)
	SchemaType string

	// Version is the version of the schema
	Version string

	// ManifestKey is the full key in format "schema@version"
	ManifestKey string
}

// temporaryManifest is used internally to extract only the manifest field from YAML
type temporaryManifest struct {
	Manifest string `yaml:"manifest"`
}
