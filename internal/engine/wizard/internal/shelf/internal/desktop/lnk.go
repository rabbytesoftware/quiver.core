package desktop

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/fsguard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/ownership"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/platform"
)

const (
	lnkMarker    = "quiver:"
	lnkSeparator = "|"
	lnkExtension = ".lnk"
	lnkIconExt   = ".ico"

	lnkEnvPath   = "QUIVER_LNK_PATH"
	lnkEnvTarget = "QUIVER_LNK_TARGET"
	lnkEnvIcon   = "QUIVER_LNK_ICON"
	lnkEnvDesc   = "QUIVER_LNK_DESC"

	lnkCreateScript = "$s=(New-Object -ComObject WScript.Shell).CreateShortcut($env:QUIVER_LNK_PATH);" +
		"$s.TargetPath=$env:QUIVER_LNK_TARGET;" +
		"$s.WorkingDirectory=(Split-Path -Parent $env:QUIVER_LNK_TARGET);" +
		"if($env:QUIVER_LNK_ICON){$s.IconLocation=$env:QUIVER_LNK_ICON};" +
		"$s.Description=$env:QUIVER_LNK_DESC;" +
		"$s.Save()"
	lnkDescribeScript = "(New-Object -ComObject WScript.Shell).CreateShortcut($env:QUIVER_LNK_PATH).Description"
)

func lnkDir(
	appData string,
) string {
	return filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs", "Quiver")
}

func lnkIcon(
	req models.ApplyRequest,
	entry domain.ExposeEntry,
	c models.Candidate,
) string {
	icon := desktopIcon(req, entry, c)
	if !fsguard.HasSuffixFold(icon, lnkIconExt) {
		return ""
	}
	return icon
}

func (p *placer) powershell(
	ctx context.Context,
	script string,
	env []string,
) ([]byte, error) {
	return p.host.Commander.Run(ctx, env, "powershell", "-NoProfile", "-NonInteractive", "-Command", script)
}

func (p *placer) placeLnk(
	ctx context.Context,
	req models.ApplyRequest,
	entry domain.ExposeEntry,
	c models.Candidate,
) (models.Placement, error) {
	reason, err := fsguard.RequireTarget(c.Target, false)
	if err != nil || reason != "" {
		return models.Placement{Refused: reason}, err
	}

	dir := lnkDir(p.host.AppData(req.Layout.UserHome))
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return models.Placement{}, fmt.Errorf("create %s: %w", dir, err)
	}

	loc := filepath.Join(dir, c.Name+lnkExtension)
	h, err := p.lnkHolder(ctx, loc)
	if err != nil {
		return models.Placement{}, err
	}
	if refusal := h.Refusal(req.Bare); refusal != "" {
		return models.Placement{Refused: refusal}, nil
	}

	env := []string{
		lnkEnvPath + "=" + loc,
		lnkEnvTarget + "=" + c.Target,
		lnkEnvIcon + "=" + lnkIcon(req, entry, c),
		lnkEnvDesc + "=" + lnkDescription(req.Bare, req.Workdir),
	}
	if out, err := p.powershell(ctx, lnkCreateScript, env); err != nil {
		return models.Placement{}, fmt.Errorf("create shortcut %s: %w: %s", loc, err, strings.TrimSpace(string(out)))
	}
	return models.Placement{Location: loc}, nil
}

func (p *placer) lnkHolder(
	ctx context.Context,
	loc string,
) (ownership.Holder, error) {
	_, err := os.Lstat(loc)
	if errors.Is(err, fs.ErrNotExist) {
		return ownership.Holder{}, nil
	}
	if err != nil {
		return ownership.Holder{}, fmt.Errorf("inspect %s: %w", loc, err)
	}

	out, err := p.powershell(ctx, lnkDescribeScript, []string{lnkEnvPath + "=" + loc})
	if err != nil {
		return ownership.Holder{}, fmt.Errorf("describe shortcut %s: %w", loc, err)
	}

	owner, found := strings.CutPrefix(strings.TrimSpace(string(out)), lnkMarker)
	if !found {
		return ownership.Holder{Exists: true}, nil
	}
	bare, workdir, _ := strings.Cut(owner, lnkSeparator)
	return ownership.Holder{Exists: true, Namespace: domain.Namespace(bare), Target: workdir}, nil
}

func lnkDescription(
	bare domain.Namespace,
	workdir string,
) string {
	return lnkMarker + string(bare) + lnkSeparator + workdir
}

func (p *placer) removeLnks(
	ctx context.Context,
	l platform.Layout,
	claim ownership.Claim,
	keep map[string]bool,
) error {
	dir := lnkDir(p.host.AppData(l.UserHome))
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("list %s: %w", dir, err)
	}

	var errs []error
	for _, e := range entries {
		loc := filepath.Join(dir, e.Name())
		if keep[loc] || !e.Type().IsRegular() || !fsguard.HasSuffixFold(e.Name(), lnkExtension) {
			continue
		}
		errs = append(errs, p.removeLnk(ctx, loc, claim))
	}
	return errors.Join(errs...)
}

func (p *placer) removeLnk(
	ctx context.Context,
	loc string,
	claim ownership.Claim,
) error {
	h, err := p.lnkHolder(ctx, loc)
	if err != nil {
		return err
	}
	if !claim(h) {
		return nil
	}
	if err := os.Remove(loc); err != nil {
		return fmt.Errorf("remove %s: %w", loc, err)
	}
	return nil
}
