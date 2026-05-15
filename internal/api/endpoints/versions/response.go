package versions

type versionsResponse struct {
	Version string  `json:"version"`
	BuildID string  `json:"build_id"`
	API     apiInfo `json:"api"`
}

type apiInfo struct {
	Supported []string `json:"supported"`
	Latest    string   `json:"latest"`
}
