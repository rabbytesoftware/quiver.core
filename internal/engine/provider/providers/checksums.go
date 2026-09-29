package providers

import (
	"crypto/sha256"
	"encoding/hex"
	"path"
	"strings"
)

const (
	maxChecksumBytes = 1 << 20
	sha256Prefix     = "sha256:"
	singleSumSuffix  = ".sha256"
)

func otherAlgorithms() []string {
	return []string{"b3sum", ".b3", "blake", "sha1", "sha3", "sha224", "sha384", "sha512", "md5"}
}

func sha256Digest(
	value string,
) string {
	lower := strings.ToLower(strings.TrimSpace(value))
	decoded, err := hex.DecodeString(lower)
	if err != nil || len(decoded) != sha256.Size {
		return ""
	}
	return sha256Prefix + lower
}

func checksumTarget(
	name string,
) (string, bool) {
	lower := strings.ToLower(name)
	if namesOtherAlgorithm(lower) {
		return "", false
	}
	if target, ok := strings.CutSuffix(lower, singleSumSuffix); ok && target != "" {
		return target, true
	}
	return "", isChecksumList(lower)
}

func namesOtherAlgorithm(
	lower string,
) bool {
	for _, algorithm := range otherAlgorithms() {
		if strings.Contains(lower, algorithm) {
			return true
		}
	}
	return false
}

func isChecksumList(
	lower string,
) bool {
	if lower == "sha256sums" || lower == "sha256sums.txt" {
		return true
	}
	return strings.Contains(lower, "checksums") && strings.HasSuffix(lower, ".txt")
}

func parseChecksums(
	sums *checksumSet,
	body []byte,
	target string,
) {
	for _, line := range strings.Split(string(body), "\n") {
		name, digest, ok := parseChecksumLine(line, target)
		if ok {
			sums.add(name, digest)
		}
	}
}

func parseChecksumLine(
	line string,
	target string,
) (string, string, bool) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return "", "", false
	}

	digest := sha256Digest(fields[0])
	if digest == "" {
		return "", "", false
	}

	name := entryName(fields[1:])
	if target == "" {
		return name, digest, name != ""
	}
	if name != "" && name != target {
		return "", "", false
	}
	return target, digest, true
}

func entryName(
	fields []string,
) string {
	entry := strings.TrimPrefix(strings.Join(fields, " "), "*")
	if entry == "" {
		return ""
	}
	return strings.ToLower(path.Base(entry))
}

type checksumSet struct {
	digests    map[string]string
	conflicted map[string]struct{}
}

func newChecksumSet() *checksumSet {
	return &checksumSet{
		digests:    make(map[string]string),
		conflicted: make(map[string]struct{}),
	}
}

func (s *checksumSet) add(
	name string,
	digest string,
) {
	if _, bad := s.conflicted[name]; bad {
		return
	}
	existing, seen := s.digests[name]
	if !seen {
		s.digests[name] = digest
		return
	}
	if existing != digest {
		delete(s.digests, name)
		s.conflicted[name] = struct{}{}
	}
}

func (s *checksumSet) lookup(
	names []string,
) string {
	found := ""
	for _, name := range names {
		digest, ok := s.digests[name]
		if !ok || digest == found {
			continue
		}
		if found != "" {
			return ""
		}
		found = digest
	}
	return found
}
