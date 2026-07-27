package step

import "strings"

// varOpen begins a Quiver reference. Nothing else in a step field does — a bare
// $NAME is the shell's, whatever it names.
const varOpen = "${"

// Expand substitutes every ${...} reference in raw with the matching value from
// r.Vars, leaving every other byte for the shell.
//
// ${...} is the manifest's syntax for injecting Quiver's own variables, not an
// environment-variable mechanism: $HOME, $(cmd), backticks and "$@" are handed
// through untouched. A ${...} whose name r.Vars does not carry is left verbatim,
// so a typo stays visible instead of silently collapsing to an empty string.
func (r Request) Expand(
	raw string,
) string {
	var out strings.Builder
	out.Grow(len(raw))

	rest := raw
	for {
		start := strings.Index(rest, varOpen)
		if start < 0 {
			out.WriteString(rest)
			return out.String()
		}

		out.WriteString(rest[:start])
		body := rest[start+len(varOpen):]

		end := strings.IndexByte(body, '}')
		if end < 0 {
			out.WriteString(rest[start:])
			return out.String()
		}

		if value, ok := r.Vars[body[:end]]; ok {
			out.WriteString(value)
		} else {
			out.WriteString(rest[start : start+len(varOpen)+end+1])
		}

		rest = body[end+1:]
	}
}
