package console

type execExitFrame struct {
	Type  string `json:"type" yaml:"type"`
	Code  int    `json:"code" yaml:"code"`
	Error string `json:"error" yaml:"error"`
}
