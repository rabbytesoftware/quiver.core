package arrow

import "fmt"

type ActionType string

const (
	ActionTypeRun        ActionType = "run"
	ActionTypeDownload   ActionType = "download"
	ActionTypeCopy       ActionType = "copy"
	ActionTypeUncompress ActionType = "uncompress"
)

const (
	MaxTimeoutDuration = "24h"
	MinPlatforms       = 1
)

type Action struct {
	Type          ActionType `json:"type"`
	Value         string     `json:"value"`
	Name          string     `json:"name"`
	To            string     `json:"to"`
	ExitOnFailure bool       `json:"exit_on_failure"`
	Timeout       string     `json:"timeout"`
}

func (a *Action) Validate() error {
	if a.Type == "" {
		return fmt.Errorf("action type cannot be empty")
	}

	switch a.Type {
	case ActionTypeRun, ActionTypeDownload, ActionTypeCopy, ActionTypeUncompress:
	default:
		return fmt.Errorf("invalid action type: %s", a.Type)
	}

	if a.Value == "" {
		return fmt.Errorf("action value cannot be empty for type %s", a.Type)
	}

	if (a.Type == ActionTypeDownload || a.Type == ActionTypeCopy || a.Type == ActionTypeUncompress) && a.To == "" {
		return fmt.Errorf("action type %s requires 'to' destination", a.Type)
	}

	return nil
}

type Method struct {
	Platforms    []string `json:"platforms"`
	Dependencies []string `json:"dependencies"`
	Workdir      string   `json:"workdir"`
	MethodName   string   `json:"method_name"`
	Actions      []Action `json:"actions"`
}

func (m *Method) Validate() error {
	if len(m.Platforms) < MinPlatforms {
		return fmt.Errorf("method must support at least %d platform(s)", MinPlatforms)
	}

	if m.MethodName == "" {
		return fmt.Errorf("method name cannot be empty")
	}

	if len(m.Actions) == 0 {
		return fmt.Errorf("method must have at least one action")
	}

	for i, action := range m.Actions {
		if err := action.Validate(); err != nil {
			return fmt.Errorf("action[%d] validation failed: %w", i, err)
		}
	}

	return nil
}
