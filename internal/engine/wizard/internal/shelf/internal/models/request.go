package models

import (
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type Layout struct {
	Bin        string
	Namespaces string
	UserHome   string
	AppData    string
	Apps       []string
}

type Request struct {
	Layout  Layout
	Bare    domain.Namespace
	Workdir string
	Media   domain.ArrowMedia
	Moved   map[string]string
}
