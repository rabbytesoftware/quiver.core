package console

type execOutFrame struct {
	Type   string `json:"type" yaml:"type"`
	Stream string `json:"stream" yaml:"stream"`
	Data   string `json:"data" yaml:"data"`
}
