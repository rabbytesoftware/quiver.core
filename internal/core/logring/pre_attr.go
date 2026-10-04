package logring

import "log/slog"

type preAttr struct {
	prefix string
	attr   slog.Attr
}
