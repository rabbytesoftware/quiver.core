package netbridge

type InfrastructureStatus struct {
	Enabled     bool     `json:"enabled"`
	Available   []string `json:"available"`
	Selected    string   `json:"selected"`
	HasFallback bool     `json:"has_fallback"`
}
