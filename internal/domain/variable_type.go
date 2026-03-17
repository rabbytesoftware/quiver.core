package domain

type VariableType string

const (
	VariableTypeString  VariableType = "string"
	VariableTypeNumber  VariableType = "number"
	VariableTypeBoolean VariableType = "boolean"
)

func (v VariableType) String() string {
	return string(v)
}

func (v *VariableType) IsValid() bool {
	return *v == VariableTypeString || *v == VariableTypeNumber || *v == VariableTypeBoolean
}

func (v *VariableType) IsString() bool {
	return *v == VariableTypeString
}

func (v *VariableType) IsNumber() bool {
	return *v == VariableTypeNumber
}

func (v *VariableType) IsBoolean() bool {
	return *v == VariableTypeBoolean
}
