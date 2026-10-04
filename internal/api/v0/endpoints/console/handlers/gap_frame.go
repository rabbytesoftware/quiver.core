package console

type gapFrame struct {
	Type    string `json:"type" yaml:"type"`
	Dropped uint64 `json:"dropped" yaml:"dropped"`
}
