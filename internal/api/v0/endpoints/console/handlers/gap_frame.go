package console

type gapFrame struct {
	Type    string `json:"type"`
	Dropped uint64 `json:"dropped"`
}
