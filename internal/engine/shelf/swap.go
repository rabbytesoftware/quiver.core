package shelf

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

const (
	stagedSuffix  = ".quiver-new"
	reservedInfix = ".quiver-"
)

func swapFile(
	loc string,
	data []byte,
) error {
	staged := loc + stagedSuffix
	if err := clearStaged(staged); err != nil {
		return err
	}
	if err := os.WriteFile(staged, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", staged, err)
	}
	if err := os.Rename(staged, loc); err != nil {
		return fmt.Errorf("swap %s: %w", loc, err)
	}
	return nil
}

func swapSymlink(
	target string,
	loc string,
) error {
	staged := loc + stagedSuffix
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
