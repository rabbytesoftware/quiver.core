package recommendation

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const day = 24 * time.Hour

// parseWindow reads a recency window: a Go duration, or a whole number of days
// written as 90d, which Go durations cannot express. Empty means no window.
func parseWindow(
	raw string,
) (time.Duration, error) {
	if raw == "" {
		return 0, nil
	}

	if days, ok := strings.CutSuffix(raw, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("window %q: want a non-negative number of days", raw)
		}
		return time.Duration(n) * day, nil
	}

	window, err := time.ParseDuration(raw)
	if err != nil || window < 0 {
		return 0, fmt.Errorf("window %q: want a non-negative duration", raw)
	}
	return window, nil
}
