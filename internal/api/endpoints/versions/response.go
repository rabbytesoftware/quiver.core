package versions

type versionsResponse struct {
	Version  string   `json:"version"`
	BuildID  string   `json:"build_id"`
	Commit   string   `json:"commit"`
	BuiltAt  string   `json:"built_at"`
	Channel  string   `json:"channel"`
	Features []string `json:"features"`
	API      apiInfo  `json:"api"`
}

type apiInfo struct {
	Supported []string `json:"supported"`
	Latest    string   `json:"latest"`
}
