package picker

import (
	"regexp"
)

type pattern struct {
	label string
	re    *regexp.Regexp
}

func bounded(
	source string,
) *regexp.Regexp {
	return regexp.MustCompile(boundaryStart + "(?:" + source + ")" + boundaryEnd)
}

func suffixed(
	suffix string,
) *regexp.Regexp {
	return regexp.MustCompile(regexp.QuoteMeta(suffix) + "$")
}

func firstMatch(
	patterns []pattern,
	name string,
) (string, bool) {
	for _, p := range patterns {
		if p.re.MatchString(name) {
			return p.label, true
		}
	}
	return "", false
}

func allMatches(
	patterns []pattern,
	name string,
) []string {
	var labels []string
	for _, p := range patterns {
		if p.re.MatchString(name) {
			labels = append(labels, p.label)
		}
	}
	return labels
}
