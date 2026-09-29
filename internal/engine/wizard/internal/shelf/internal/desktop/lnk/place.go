package lnk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/discover"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/fsguard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/ownership"
)

const (
	ownerPrefix  = "quiver:"
	separator    = "|"
	fileExt      = ".lnk"
	iconExt      = ".ico"
	maxLinkBytes = 1 << 20
)

type exposer struct {
	scan discover.Scan
}

func New(
	scan discover.Scan,
) models.Exposer {
	return &exposer{scan: scan}
}

func (e *exposer) Place(
	_ context.Context,
	req models.Request,
	entry domain.ExposeEntry,
	c models.Candidate,
) (models.Placement, error) {
	reason, err := fsguard.RequireTarget(c.Target, false)
	if err != nil || reason != "" {
		return models.Placement{Refused: reason}, err
	}

	dir := startMenuDir(req.Layout.AppData)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return models.Placement{}, fmt.Errorf("create %s: %w", dir, err)
	}

	loc := filepath.Join(dir, c.Name+fileExt)
	h, err := holder(loc)
	if err != nil {
		return models.Placement{}, err
	}
	if refusal := h.Refusal(req.Bare); refusal != "" {
		return models.Placement{Refused: refusal}, nil
	}

	data, err := Encode(Link{
		Target:      c.Target,
		WorkingDir:  filepath.Dir(c.Target),
		Description: description(req.Bare, req.Workdir),
		Icon:        icon(req, entry, c),
	})
	if err != nil {
		return models.Placement{}, fmt.Errorf("encode %s: %w", loc, err)
	}
	if err := fsguard.SwapFile(loc, data); err != nil {
		return models.Placement{}, err
	}
	return models.Placement{Location: loc}, nil
}

func (e *exposer) Remove(
	_ context.Context,
	l models.Layout,
	claim models.Claim,
	keep map[string]bool,
) error {
	dir := startMenuDir(l.AppData)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("list %s: %w", dir, err)
	}

	var errs []error
	for _, d := range entries {
		loc := filepath.Join(dir, d.Name())
		if keep[loc] || !d.Type().IsRegular() || !fsguard.HasSuffixFold(d.Name(), fileExt) {
			continue
		}
		errs = append(errs, removeClaimed(loc, claim))
	}
	return errors.Join(errs...)
}

func removeClaimed(
	loc string,
	claim models.Claim,
) error {
	h, err := holder(loc)
	if err != nil || !h.ClaimedBy(claim) {
		return err
	}
	if err := os.Remove(loc); err != nil {
		return fmt.Errorf("remove %s: %w", loc, err)
	}
	return nil
}

// holder reads a shortcut's owner from its description. A shortcut that does
// not parse, from whatever tool, is simply not Quiver's.
func holder(
	loc string,
) (ownership.Holder, error) {
	info, err := os.Lstat(loc)
	if errors.Is(err, fs.ErrNotExist) {
		return ownership.Holder{}, nil
	}
	if err != nil {
		return ownership.Holder{}, fmt.Errorf("inspect %s: %w", loc, err)
	}
	if !info.Mode().IsRegular() {
		return ownership.Holder{Exists: true}, nil
	}

	data, err := readLink(loc)
	if err != nil {
		return ownership.Holder{}, err
	}
	link, err := Decode(data)
	if err != nil {
		return ownership.Holder{Exists: true}, nil
	}

	owner, found := strings.CutPrefix(strings.TrimSpace(link.Description), ownerPrefix)
	if !found {
		return ownership.Holder{Exists: true}, nil
	}
	bare, workdir, _ := strings.Cut(owner, separator)
	return ownership.Holder{Exists: true, Namespace: domain.Namespace(bare), Target: workdir}, nil
}

func readLink(
	loc string,
) ([]byte, error) {
	f, err := os.Open(loc) // #nosec G304 -- loc is a shortcut in the Quiver Start Menu folder being inspected for its owner
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", loc, err)
	}
	defer func() { _ = f.Close() }()

	data, err := io.ReadAll(io.LimitReader(f, maxLinkBytes))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", loc, err)
	}
	return data, nil
}

func description(
	bare domain.Namespace,
	workdir string,
) string {
	return ownerPrefix + string(bare) + separator + workdir
}

func startMenuDir(
	appData string,
) string {
	return filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs", "Quiver")
}

func icon(
	req models.Request,
	entry domain.ExposeEntry,
	c models.Candidate,
) string {
	path := discover.Icon(req, entry, c)
	if !fsguard.HasSuffixFold(path, iconExt) {
		return ""
	}
	return path
}
