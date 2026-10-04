package step

import "encoding/json"

// UIStep opens an arrow surface for the method that declares it. Exactly one
// of Listen or Static is set (enforced by the ui_step rule, not here).
type UIStep struct {
	BasicStep
	// Listen lists the transports the arrow can serve on ("unix", "pipe").
	Listen []string `json:"listen,omitempty"`
	// Static is a directory, relative to the install path, the daemon serves.
	Static string `json:"static,omitempty"`
	// Path is the initial path, default "/".
	Path string `json:"path,omitempty"`
}

// Resolve is the identity: a ui step has no Overrideable fields in v0.
func (s UIStep) Resolve(string) Step { return s }

func NewUIStep(
	title string,
	listen []string,
	static string,
	path string,
	exitOnFailure bool,
) UIStep {
	return UIStep{
		BasicStep: newBasicStep(StepTypeUI, title, exitOnFailure),
		Listen:    listen,
		Static:    static,
		Path:      path,
	}
}

type uiWire struct {
	Kind          StepType `json:"type"`
	Title         string   `json:"title"`
	ExitOnFailure bool     `json:"exit_on_failure"`
	Listen        []string `json:"listen,omitempty"`
	Static        string   `json:"static,omitempty"`
	Path          string   `json:"path,omitempty"`
}

func (s UIStep) MarshalJSON() ([]byte, error) {
	return json.Marshal(uiWire{
		Kind:          s.Type(),
		Title:         s.Title(),
		ExitOnFailure: s.ExitOnFailure(),
		Listen:        s.Listen,
		Static:        s.Static,
		Path:          s.Path,
	})
}

func (s *UIStep) UnmarshalJSON(data []byte) error {
	var wire uiWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	s.BasicStep = newBasicStep(wire.Kind, wire.Title, wire.ExitOnFailure)
	s.Listen = wire.Listen
	s.Static = wire.Static
	s.Path = wire.Path
	return nil
}
