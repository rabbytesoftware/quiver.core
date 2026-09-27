package portable

import (
	"fmt"
	"os"
)

func removeSource(
	workDir string,
	from string,
	output string,
) error {
	if from == output {
		return nil
	}

	if _, inside := workDirRel(workDir, from); !inside {
		return nil
	}

	if err := os.Remove(from); err != nil {
		return fmt.Errorf("portable: remove source %s: %w", from, err)
	}

	return nil
}
