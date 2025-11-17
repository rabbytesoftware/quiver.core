package validator

type Validator interface {
	Validate(schemaJSON []byte, yamlData []byte) (*ValidationResult, error)
}

type ValidationResult struct {
	Valid  bool
	Errors []ValidationError
}

type ValidationError struct {
	Field   string
	Message string
	Value   interface{}
}
