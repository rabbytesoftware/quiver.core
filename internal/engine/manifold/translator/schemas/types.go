package schemas

type Mapper[T any] interface {
	Map(yamlData map[string]interface{}) (*T, error)

	GetSchema() ([]byte, error)
}
