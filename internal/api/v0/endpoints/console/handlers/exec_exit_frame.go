package console

type execExitFrame struct {
	Type  string `json:"type"`
	Code  int    `json:"code"`
	Error string `json:"error"`
}
