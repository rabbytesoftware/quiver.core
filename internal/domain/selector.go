package domain

// SelectorKind names the type of selector a catalog row tracks.
type SelectorKind string

// A kind is stored on the row when it is created, and the refined kinds say
// which ref the selector named at that moment, so a later tag or branch of
// the same name never changes what the row follows. The unrefined pin and
// channel are what rows written before the refinement carry.
const (
	// SelectorPin is the zero value: a row with no stored kind is a pin of
	// its tag, else of its branch.
	SelectorPin SelectorKind = ""
	// SelectorTagPin follows exactly one tag.
	SelectorTagPin SelectorKind = "pin:tag"
	// SelectorBranchPin follows exactly one branch.
	SelectorBranchPin SelectorKind = "pin:branch"
	// SelectorChannel tracks the newest ref of a release channel: an ordered
	// channel of its name, else a rolling tag, else the HEAD branch.
	SelectorChannel SelectorKind = "channel"
	// SelectorOrderedChannel tracks the newest member of a classified channel.
	SelectorOrderedChannel SelectorKind = "channel:ordered"
	// SelectorPointerChannel tracks one rolling tag.
	SelectorPointerChannel SelectorKind = "channel:pointer"
	// SelectorBranchChannel tracks the HEAD branch a tagless repository
	// was added from.
	SelectorBranchChannel SelectorKind = "channel:branch"
	// SelectorConstraint tracks the newest tag satisfying a semver constraint.
	SelectorConstraint SelectorKind = "constraint"
	// SelectorCommit pins one commit, named by its full or abbreviated hash.
	SelectorCommit SelectorKind = "commit"
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
	Head     string            `json:"head,omitempty" yaml:"head,omitempty"`
}

// Valid returns true if the selector kind is one of the defined constants.
func (k SelectorKind) Valid() bool {
	switch k {
	case SelectorPin, SelectorTagPin, SelectorBranchPin,
		SelectorChannel, SelectorOrderedChannel, SelectorPointerChannel, SelectorBranchChannel,
		SelectorConstraint, SelectorCommit:
		return true
	default:
		return false
	}
}

// Family returns the kind a refined kind belongs to: pin, channel,
// constraint or commit.
func (k SelectorKind) Family() SelectorKind {
	switch k {
	case SelectorPin, SelectorTagPin, SelectorBranchPin:
		return SelectorPin
	case SelectorChannel, SelectorOrderedChannel, SelectorPointerChannel, SelectorBranchChannel:
		return SelectorChannel
	case SelectorConstraint, SelectorCommit:
		return k
	default:
		return k
	}
}

// Commit looks up a ref in the snapshot, preferring tags over branches.
func (s RefSnapshot) Commit(ref string) (string, bool) {
	if commit, ok := s.Tags[ref]; ok {
		return commit, true
	}
	commit, ok := s.Branches[ref]
	return commit, ok
}
