package bind

import (
	"fmt"
	"regexp"
)

func matcher(
	pattern string,
) (func(string) bool, error) {
	if regexp.QuoteMeta(pattern) == pattern {
		return func(name string) bool { return name == pattern }, nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("bind: pattern %q: %w", pattern, err)
	}
	return re.MatchString, nil
}
