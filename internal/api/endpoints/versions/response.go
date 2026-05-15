package versions

const minClientVersion = "1.0.0"

type versionsResponse struct {
	Version string  `json:"version"`
	BuildID string  `json:"build_id"`
	API     apiInfo `json:"api"`
}

type apiInfo struct {
	Supported        []string `json:"supported"`
	Latest           string   `json:"latest"`
	MinClientVersion string   `json:"min_client_version"`
}
