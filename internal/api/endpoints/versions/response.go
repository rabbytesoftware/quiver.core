package versions

// Build is the identity of the running daemon: the values the release pipeline
// stamps into the binary and the optional features it serves.
type Build struct {
	Version  string   `json:"version"`
	BuildID  string   `json:"build_id"`
	Commit   string   `json:"commit"`
	BuiltAt  string   `json:"built_at"`
	Channel  string   `json:"channel"`
	Features []string `json:"features"`
}

type versionsResponse struct {
	Build
	API apiInfo `json:"api"`
}

type apiInfo struct {
	Supported []string `json:"supported"`
	Latest    string   `json:"latest"`
}
