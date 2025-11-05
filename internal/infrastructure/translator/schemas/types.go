package schemas

type Mapper[T any] interface {
	// Map converts YAML data to a domain model
	Map(yamlData map[string]interface{}) (*T, error)

	// GetSchema returns the JSON schema for validation
	GetSchema() ([]byte, error)
}
