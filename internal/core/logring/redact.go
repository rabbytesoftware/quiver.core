package logring

import (
	"regexp"
	"strings"
)

// Redacted replaces every sensitive value the ring stores or an audit line
// carries.
const Redacted = "[redacted]"

var (
	sensitiveKey = regexp.MustCompile(
		`(?i)(token|secret|password|passwd|authorization|api[-_]?key|private[-_]?key|bearer|cookie|credential|pairing)`,
	)
	bearerValue = regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~+/=-]+`)
	userinfo    = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)[^/\s:@]+:[^/\s@]+@`)
)

// IsSensitiveKey reports whether an attribute or flag name marks its value as
// a secret: it contains token, secret, password, passwd, authorization,
// apikey, api_key, bearer, cookie, credential or pairing, in any case.
func IsSensitiveKey(
	key string,
) bool {
	return sensitiveKey.MatchString(key)
}

// RedactValue returns Redacted when key is sensitive. A string value that is
// not under a sensitive key still has any bearer credential or URL userinfo
// inside it replaced. Other values pass through unchanged.
func RedactValue(
	key string,
	value any,
) any {
	if IsSensitiveKey(key) {
		return Redacted
	}

	text, ok := value.(string)
	if !ok {
		return value
	}
	return scrub(text)
}

func scrub(
	text string,
) string {
	if mentionsBearer(text) {
		text = bearerValue.ReplaceAllString(text, "${1}"+Redacted)
	}
	if strings.Contains(text, "://") {
		text = userinfo.ReplaceAllString(text, "${1}"+Redacted+"@")
	}
	return text
}

func mentionsBearer(
	text string,
) bool {
	return strings.Contains(strings.ToLower(text), "bearer")
}

// RedactLine returns a command line with every secret-looking value replaced,
// for the audit record. Whitespace runs collapse to single spaces.
//
// A token of the form key=value is redacted when key (without leading dashes)
// is sensitive, and so is the token after a bare sensitive flag.
func RedactLine(
	line string,
) string {
	fields := strings.Fields(line)
	redactNext := false
	for i, field := range fields {
		fields[i], redactNext = redactField(field, redactNext)
	}
	return strings.Join(fields, " ")
}

func redactField(
	field string,
	redactThis bool,
) (string, bool) {
	if redactThis {
		return Redacted, false
	}

	key, _, hasValue := strings.Cut(strings.TrimLeft(field, "-"), "=")
	if !IsSensitiveKey(key) {
		return redactAssignment(field, hasValue), false
	}
	if hasValue {
		return field[:strings.Index(field, "=")+1] + Redacted, false
	}
	return field, true
}

func redactAssignment(
	field string,
	hasValue bool,
) string {
	if !hasValue {
		return scrub(field)
	}

	head, value, _ := strings.Cut(field, "=")
	inner, _ := redactField(value, false)
	return head + "=" + inner
}
