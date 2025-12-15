package commands

import (
	"context"
	"errors"

	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
	"github.com/rabbytesoftware/quiver/internal/infrastructure/wizard/events"
	"github.com/rabbytesoftware/quiver/internal/models/arrow"
	"github.com/rabbytesoftware/quiver/internal/repositories/arrows"
)

type AddArrowCommand struct {
	Arrow   *arrow.Arrow
	Source  string
	AddedBy string
}

type AddArrow struct {
	Command   AddArrowCommand
	arrowRepo arrows.ArrowsInterface
}

func NewAddArrow(
	cmd AddArrowCommand,
	arrowRepo arrows.ArrowsInterface,
) *AddArrow {
	return &AddArrow{
		Command:   cmd,
		arrowRepo: arrowRepo,
	}
}

func (c *AddArrow) Validate(ctx context.Context) error {
	if c.Command.Arrow == nil {
		return errors.New("invalid arrow data")
	}

	existing := c.arrowRepo.GetById(c.Command.Arrow.ID.String())
	if existing != nil {
		return errors.New("arrow already exists")
	}

	return nil
}

func (c *AddArrow) ToEvent() domain.Event {
	return events.NewArrowAdded(
		c.Command.Arrow.ID.String(),
		c.Command.Arrow,
		c.Command.Source,
		c.Command.AddedBy,
	)
}
