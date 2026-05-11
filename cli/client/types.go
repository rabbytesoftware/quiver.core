package client

// ArrowRuntime is the full runtime state snapshot pushed by the WS stream.
// Mirrors the server's ArrowRuntimeDTO.
type ArrowRuntime struct {
	Namespace  string
	State      string
	ActiveRun  *RunRecord
	LastReturn *Return
}

// RunRecord holds the current execution context.
// Mirrors the server's RunRecordDTO.
type RunRecord struct {
	Method    string
	PID       int
	Variables map[string]string
	Steps     []StepProgress
}

// StepProgress is the status of one execution step.
// Mirrors the server's StepProgressDTO.
type StepProgress struct {
	Index  int
	Status string // pending | running | completed | failed
	Title  string
	Type   string
	Error  *string
}

// Return holds the result of a completed execution.
// Mirrors the server's ReturnDTO.
type Return struct {
	Method    string
	Outcome   string // success | failed | cancelled
	Variables map[string]string
	Steps     []StepProgress
}

// ArrowListItem is one entry in the arrow catalog list.
// Mirrors the server's ArrowListItemDTO.
type ArrowListItem struct {
	Namespace   string
	Name        string
	Description string
	Tags        []string
	Versions    []InstalledVersion
}

// InstalledVersion is one installed ref of an arrow.
// Mirrors the server's InstalledVersionItemDTO.
type InstalledVersion struct {
	Ref         string
	Version     string
	State       string
	InstalledAt string
	Constraint  string
}

// ArrowDetail is the full detail view of a catalog entry.
// Mirrors the server's ArrowDetailDTO.
type ArrowDetail struct {
	Namespace           string
	Name                string
	Version             string
	Description         string
	License             string
	State               string
	Tags                []string
	InstalledRef        string
	InstalledAt         string
	InstalledConstraint string
	UserInstalled       bool
	ActiveRun           *RunRecord
	LastReturn          *Return
}

// ValidationResult is the response from the manifest validate endpoint.
// Mirrors the server's ValidationResultDTO.
type ValidationResult struct {
	Valid                bool
	Errors               []ValidationError
	SupportedPlatforms   []string
	UnsupportedPlatforms []string
}

// ValidationError is one validation failure.
// Mirrors the server's ValidationErrorDTO.
type ValidationError struct {
	Field   string
	Rule    string
	Message string
}

// Collection is a Quiver collection (renamed from "Quiver" per CLI spec v2 changelog).
// Mirrors the server's QuiverListItemDTO.
type Collection struct {
	Namespace   string
	Name        string
	Description string
	Tags        []string
}

// HealthStatus is the response from the health endpoint.
type HealthStatus struct {
	Status string
}
