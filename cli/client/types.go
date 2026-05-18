package client

// ArrowRuntime is the full runtime state snapshot pushed by the WS stream.
// Mirrors the server's ArrowRuntimeDTO.
type ArrowRuntime struct {
	Namespace  string     `json:"namespace"`
	State      string     `json:"state"`
	ActiveRun  *RunRecord `json:"active_run,omitempty"`
	LastReturn *Return    `json:"last_return,omitempty"`
}

// RunRecord holds the current execution context.
// Mirrors the server's RunRecordDTO.
type RunRecord struct {
	Method    string            `json:"method"`
	PID       int               `json:"pid,omitempty"`
	Variables map[string]string `json:"variables,omitempty"`
	Steps     []StepProgress    `json:"steps,omitempty"`
}

// StepProgress is the status of one execution step.
// Mirrors the server's StepProgressDTO.
type StepProgress struct {
	Index  int     `json:"index"`
	Status string  `json:"status"` // pending | running | completed | failed
	Title  string  `json:"title"`
	Type   string  `json:"type"`
	Error  *string `json:"error,omitempty"`
}

// Return holds the result of a completed execution.
// Mirrors the server's ReturnDTO.
type Return struct {
	Method    string            `json:"method"`
	Outcome   string            `json:"outcome"` // success | failed | cancelled
	Variables map[string]string `json:"variables,omitempty"`
	Steps     []StepProgress    `json:"steps,omitempty"`
}

// ArrowListItem is one entry in the arrow catalog list.
// Mirrors the server's ArrowListItemDTO.
type ArrowListItem struct {
	Namespace   string             `json:"namespace"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Tags        []string           `json:"tags"`
	Versions    []InstalledVersion `json:"versions"`
}

// InstalledVersion is one installed ref of an arrow.
// Mirrors the server's InstalledVersionItemDTO.
type InstalledVersion struct {
	Ref         string `json:"ref"`
	Version     string `json:"version"`
	State       string `json:"state"`
	InstalledAt string `json:"installed_at"`
	Constraint  string `json:"constraint,omitempty"`
}

// ArrowDetail is the full detail view of a catalog entry.
// Mirrors the server's ArrowDetailDTO.
type ArrowDetail struct {
	Namespace           string     `json:"namespace"`
	Name                string     `json:"name"`
	Version             string     `json:"version"`
	Description         string     `json:"description"`
	License             string     `json:"license"`
	State               string     `json:"state"`
	Tags                []string   `json:"tags"`
	InstalledRef        string     `json:"installed_ref,omitempty"`
	InstalledAt         string     `json:"installed_at,omitempty"`
	InstalledConstraint string     `json:"installed_constraint,omitempty"`
	UserInstalled       bool       `json:"user_installed"`
	ActiveRun           *RunRecord `json:"active_run,omitempty"`
	LastReturn          *Return    `json:"last_return,omitempty"`
}

// ValidationResult is the response from the manifest validate endpoint.
// Mirrors the server's ValidationResultDTO.
type ValidationResult struct {
	Valid                bool              `json:"valid"`
	Errors               []ValidationError `json:"errors,omitempty"`
	SupportedPlatforms   []string          `json:"supported_platforms,omitempty"`
	UnsupportedPlatforms []string          `json:"unsupported_platforms,omitempty"`
}

// ValidationError is one validation failure.
// Mirrors the server's ValidationErrorDTO.
type ValidationError struct {
	Field   string `json:"field"`
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

// Collection is a Quiver collection (renamed from "Quiver" per CLI spec v2 changelog).
// Mirrors the server's QuiverListItemDTO.
type Collection struct {
	Namespace   string   `json:"namespace"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

// HealthStatus is the response from the health endpoint.
type HealthStatus struct {
	Status string `json:"status"`
}
