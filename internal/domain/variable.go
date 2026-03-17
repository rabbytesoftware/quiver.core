package domain

import "fmt"

const (
	MaxVariableNameLength = 255
)

type Variable struct {
	Name      string       `json:"name"`
	Default   string       `json:"default"`
	Values    []string     `json:"values"`
	Min       int          `json:"min"`
	Max       int          `json:"max"`
	Sensitive bool         `json:"sensitive"`
	Type      VariableType `json:"type"`
}

func (v *Variable) Validate() error {
	if v.Name == "" {
		return fmt.Errorf("variable name cannot be empty")
	}
	if len(v.Name) > MaxVariableNameLength {
		return fmt.Errorf("variable name exceeds max length of %d", MaxVariableNameLength)
	}

	if v.Max > 0 && v.Min > v.Max {
		return fmt.Errorf("variable min (%d) cannot be greater than max (%d)", v.Min, v.Max)
	}

	if len(v.Values) > 0 {
		found := false
		for _, val := range v.Values {
			if val == v.Default {
				found = true
				break
			}
		}
		if !found && v.Default != "" {
			return fmt.Errorf("default value '%s' not found in allowed values", v.Default)
		}
	}

	return nil
}
