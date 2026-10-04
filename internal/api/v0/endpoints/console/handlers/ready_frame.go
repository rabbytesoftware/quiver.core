package console

type readyFrame struct {
	Type  string `json:"type"`
	Seq   uint64 `json:"seq"`
	Reset bool   `json:"reset,omitempty"`
}
