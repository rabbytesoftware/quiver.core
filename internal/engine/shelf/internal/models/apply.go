package models

import (
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/platform"
)

type Candidate struct {
	Name     string
	Display  string
	Target   string
	Icon     string
	Depth    int
	Declared bool
}

func (c Candidate) DisplayName() string {
	if c.Display != "" {
		return c.Display
	}
	return c.Name
}

type Placement struct {
	Location string
	Refused  string
}

type ApplyRequest struct {
	Layout  platform.Layout
	Bare    domain.Namespace
	Workdir string
	Media   domain.ArrowMedia
	Moved   map[string]string
}
