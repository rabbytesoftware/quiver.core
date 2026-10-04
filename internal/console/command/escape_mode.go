package command

type escapeMode int

const (
	escapeNone escapeMode = iota
	escapeBare
	escapeDouble
)
