package userpath

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
)

// Editor serializes every read-modify-write of one UserPath, so concurrent
// runs never lose each other's entries, and appends only: existing entries,
// their order and their spelling are kept as they are.
type Editor interface {
	Location() string
	Contains(
		dir string,
	) (bool, error)
	Append(
		ctx context.Context,
		dir string,
	) error
	Drop(
		ctx context.Context,
		drop func(entry string) bool,
	) error
}

type editor struct {
	mu   sync.Mutex
	path UserPath
}

func NewEditor(
	path UserPath,
) Editor {
	return &editor{path: path}
}

func (e *editor) Location() string {
	return e.path.Location()
}

func (e *editor) Contains(
	dir string,
) (bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	current, err := e.path.Read()
	if err != nil {
		return false, fmt.Errorf("read user path: %w", err)
	}
	return Contains(current, dir, Separator, true), nil
}

func (e *editor) Append(
	ctx context.Context,
	dir string,
) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	current, err := e.path.Read()
	if err != nil {
		return fmt.Errorf("read user path: %w", err)
	}
	if Contains(current, dir, Separator, true) {
		return nil
	}

	next := dir
	if trimmed := strings.TrimRight(current, Separator); trimmed != "" {
		next = trimmed + Separator + dir
	}
	return e.write(ctx, next)
}

func (e *editor) Drop(
	ctx context.Context,
	drop func(entry string) bool,
) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	current, err := e.path.Read()
	if err != nil {
		return fmt.Errorf("read user path: %w", err)
	}

	entries := strings.Split(current, Separator)
	kept := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry == "" || !drop(entry) {
			kept = append(kept, entry)
		}
	}
	if len(kept) == len(entries) {
		return nil
	}
	return e.write(ctx, strings.Join(kept, Separator))
}

func (e *editor) write(
	ctx context.Context,
	value string,
) error {
	if err := e.path.Write(value); err != nil {
		return fmt.Errorf("write user path: %w", err)
	}
	if err := e.path.Broadcast(); err != nil {
		slog.WarnContext(ctx, "shelf: user path change not broadcast", "err", err)
	}
	return nil
}
