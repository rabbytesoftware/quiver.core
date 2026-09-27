package shelf

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

const (
	lnkMarker    = "quiver:"
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

func (s *shelf) appData(
	userHome string,
) string {
	if s.sandboxHome != "" {
		return filepath.Join(s.sandboxHome, "AppData", "Roaming")
	}
	if v := s.env("APPDATA"); v != "" {
		return v
	}
	return filepath.Join(userHome, "AppData", "Roaming")
}

func lnkIcon(
	req applyRequest,
	entry domain.ExposeEntry,
) string {
	icon := desktopIcon(req, entry)
	if !hasSuffixFold(icon, lnkIconExt) {
		return ""
	}
	return icon
}

func (s *shelf) powershell(
	ctx context.Context,
	script string,
	env []string,
) ([]byte, error) {
	return s.commander.RunWithEnv(ctx, env, "powershell", "-NoProfile", "-NonInteractive", "-Command", script)
}

func (s *shelf) placeLnk(
	ctx context.Context,
	req applyRequest,
	entry domain.ExposeEntry,
	c candidate,
) (placement, error) {
	reason, err := requireTarget(c.target, false)
	if err != nil || reason != "" {
		return placement{refused: reason}, err
	}

	dir := lnkDir(s.appData(req.layout.userHome))
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return placement{}, fmt.Errorf("create %s: %w", dir, err)
	}

	loc := filepath.Join(dir, c.name+lnkExtension)
	h, err := s.lnkHolder(ctx, loc)
	if err != nil {
		return placement{}, err
	}
	if refusal := h.refusal(req.bare); refusal != "" {
		return placement{refused: refusal}, nil
	}

	env := []string{
		lnkEnvPath + "=" + loc,
		lnkEnvTarget + "=" + c.target,
		lnkEnvIcon + "=" + lnkIcon(req, entry),
		lnkEnvDesc + "=" + lnkMarker + string(req.bare),
	}
	if out, err := s.powershell(ctx, lnkCreateScript, env); err != nil {
		return placement{}, fmt.Errorf("create shortcut %s: %w: %s", loc, err, strings.TrimSpace(string(out)))
	}
	return placement{location: loc}, nil
}

func (s *shelf) lnkHolder(
	ctx context.Context,
	loc string,
) (holder, error) {
	_, err := os.Lstat(loc)
	if errors.Is(err, fs.ErrNotExist) {
		return holder{}, nil
	}
	if err != nil {
		return holder{}, fmt.Errorf("inspect %s: %w", loc, err)
	}

	out, err := s.powershell(ctx, lnkDescribeScript, []string{lnkEnvPath + "=" + loc})
	if err != nil {
		return holder{}, fmt.Errorf("describe shortcut %s: %w", loc, err)
	}

	owner, found := strings.CutPrefix(strings.TrimSpace(string(out)), lnkMarker)
	if !found {
		return holder{exists: true}, nil
	}
	return holder{exists: true, namespace: domain.Namespace(owner)}, nil
}

func (s *shelf) removeLnks(
	ctx context.Context,
	l layout,
	bare domain.Namespace,
	keep map[string]bool,
) error {
	dir := lnkDir(s.appData(l.userHome))
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
		if keep[loc] || !e.Type().IsRegular() || !hasSuffixFold(e.Name(), lnkExtension) {
			continue
		}
		errs = append(errs, s.removeLnk(ctx, loc, bare))
	}
	return errors.Join(errs...)
}

func (s *shelf) removeLnk(
	ctx context.Context,
	loc string,
	bare domain.Namespace,
) error {
	h, err := s.lnkHolder(ctx, loc)
	if err != nil {
		return err
	}
	if h.namespace != bare {
		return nil
	}
	if err := os.Remove(loc); err != nil {
		return fmt.Errorf("remove %s: %w", loc, err)
	}
	return nil
}
