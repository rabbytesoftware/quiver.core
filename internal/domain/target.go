package domain

type Target struct {
	Requirements Requirement       `json:"requirements"`
	Tools        []DependencyEdge  `json:"tools"`
	Services     []DependencyEdge  `json:"services"`
	Exports      map[string]string `json:"exports"`
	Lifecycle    TargetLifecycle   `json:"lifecycle"`
	Methods      map[string]Method `json:"methods"`
}
