package pathenv

import (
	"context"
	"fmt"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/host"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/userpath"
)

type windows struct {
	host   host.Host
	editor userpath.Editor
}

func NewWindows(
	h host.Host,
	editor userpath.Editor,
) models.PathManager {
	return &windows{host: h, editor: editor}
}

func (m *windows) Status(
	_ context.Context,
) (models.PathStatus, error) {
	l, err := m.host.Layout()
	if err != nil {
		return models.PathStatus{}, fmt.Errorf("shelf: path status: %w", err)
	}

	configured, err := m.editor.Contains(l.Bin)
	if err != nil {
		return models.PathStatus{}, fmt.Errorf("shelf: path status: %w", err)
	}

	return models.PathStatus{
		BinDir:     l.Bin,
		OnPath:     userpath.Contains(m.host.Env("PATH"), l.Bin, userpath.Separator, true),
		Configured: configured,
		Files:      []string{m.editor.Location()},
	}, nil
}

func (m *windows) Setup(
	ctx context.Context,
) (models.PathStatus, error) {
	l, err := m.host.Layout()
	if err != nil {
		return models.PathStatus{}, fmt.Errorf("shelf: setup path: %w", err)
	}

	if err := m.editor.Append(ctx, l.Bin); err != nil {
		return models.PathStatus{}, fmt.Errorf("shelf: setup path: %w", err)
	}
	return m.Status(ctx)
}
