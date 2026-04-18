package domain

type Target struct {
	Requirements Requirement       `json:"requirements"`
	Tools        []Namespace       `json:"tools"`
	Services     []Namespace       `json:"services"`
	Exports      map[string]string `json:"exports"`
	Lifecycle    TargetLifecycle   `json:"lifecycle"`
	Methods      map[string]Method `json:"methods"`
}
