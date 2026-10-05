package versions

type versionsResponse struct {
	Build
	API apiInfo `json:"api"`
}

type apiInfo struct {
	Supported []string `json:"supported"`
	Latest    string   `json:"latest"`
}
