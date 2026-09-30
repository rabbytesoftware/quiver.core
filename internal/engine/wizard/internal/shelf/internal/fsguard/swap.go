package fsguard

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

const (
	StagedSuffix  = ".quiver-new"
	reservedInfix = ".quiver-"
)

func SwapFile(
	loc string,
	data []byte,
) error {
	staged := loc + StagedSuffix
	if err := clearStaged(staged); err != nil {
		return err
	}
	if err := os.WriteFile(staged, data, 0o644); err != nil { // #nosec G306 -- desktop entries and shims must be readable by the launcher
		return fmt.Errorf("write %s: %w", staged, err)
	}
	if err := os.Rename(staged, loc); err != nil {
		return fmt.Errorf("swap %s: %w", loc, err)
	}
	return nil
}

func SwapSymlink(
	target string,
	loc string,
) error {
	staged := loc + StagedSuffix
	if err := clearStaged(staged); err != nil {
		return err
	}
	if err := os.Symlink(target, staged); err != nil {
		return fmt.Errorf("symlink %s: %w", staged, err)
	}
	if err := os.Rename(staged, loc); err != nil {
		return fmt.Errorf("swap %s: %w", loc, err)
	}
	return nil
}

func clearStaged(
	staged string,
) error {
	err := os.Remove(staged)
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return fmt.Errorf("clear %s: %w", staged, err)
}
