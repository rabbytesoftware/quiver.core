package schemas

// Mapper validates and unmarshals raw YAML bytes into a typed raw model T.
type Mapper[T any] interface {
	// Map unmarshals the given YAML bytes into *T.
	Map(data []byte) (*T, error)

	// GetSchema returns the JSON schema used to validate YAML before mapping.
	GetSchema() ([]byte, error)
}
