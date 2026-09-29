package domain

// SelectorKind names the type of selector a catalog row tracks.
type SelectorKind string

const (
	// SelectorPin is the zero value: a row with no stored kind is a pin.
	SelectorPin        SelectorKind = ""
	SelectorChannel    SelectorKind = "channel"
	SelectorConstraint SelectorKind = "constraint"
	SelectorCommit     SelectorKind = "commit"
)

// Resolved carries the installed ref and its resolved commit.
type Resolved struct {
	Ref         string `json:"ref" yaml:"ref"`
	Commit      string `json:"commit" yaml:"commit"`
	Fingerprint string `json:"fingerprint,omitempty" yaml:"fingerprint,omitempty"`
}

// RefOr returns the resolved ref, or fallback when nothing was resolved.
func (r Resolved) RefOr(fallback string) string {
	if r.Ref == "" {
		return fallback
	}
	return r.Ref
}

// Available carries a newly available ref and its commit.
type Available struct {
	Ref    string `json:"ref" yaml:"ref"`
	Commit string `json:"commit" yaml:"commit"`
}

// RefSnapshot carries the set of tags and branches visible on a namespace.
type RefSnapshot struct {
	Tags     map[string]string `json:"tags" yaml:"tags"`
	Branches map[string]string `json:"branches" yaml:"branches"`
	Head     string            `json:"head" yaml:"head"`
}

// Valid returns true if the selector kind is one of the four defined constants.
func (k SelectorKind) Valid() bool {
	switch k {
	case SelectorPin, SelectorChannel, SelectorConstraint, SelectorCommit:
		return true
	default:
		return false
	}
}

// Commit looks up a ref in the snapshot, preferring tags over branches.
func (s RefSnapshot) Commit(ref string) (string, bool) {
	if s.Tags != nil {
		if commit, ok := s.Tags[ref]; ok {
			return commit, true
		}
	}
	if s.Branches != nil {
		if commit, ok := s.Branches[ref]; ok {
			return commit, true
		}
	}
	return "", false
}
