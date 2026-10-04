package console

type readyFrame struct {
	Type  string `json:"type" yaml:"type"`
	Seq   uint64 `json:"seq" yaml:"seq"`
	Reset bool   `json:"reset,omitempty" yaml:"reset,omitempty"`
}
