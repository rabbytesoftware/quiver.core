package libs

// MutationResponse is the envelope returned by write operations.
type MutationResponse struct {
	Success   bool   `json:"success"`
	Namespace string `json:"namespace,omitempty"`
}

// QueryResponse is the envelope returned by read operations.
type QueryResponse struct {
	Success bool `json:"success"`
	Data    any  `json:"data,omitempty"`
}

// ErrResponse is the envelope returned on errors.
type ErrResponse struct {
	Success   bool   `json:"success"`
	Error     string `json:"error"`
	Namespace string `json:"namespace,omitempty"`
}
