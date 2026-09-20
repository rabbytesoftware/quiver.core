package domain

import "time"

type CollectionMeta struct {
	Name        string
	Description string
	URL         string
	Maintainers []string
	Tags        []string
	Media       CollectionMedia
}

type CollectionMedia struct {
	Icon   string
	Banner string
}

// Quiver aggregate — full manifest state + follow state.
type Collection struct {
	Namespace    Namespace
	FollowedAt   time.Time
	FailedArrows []Namespace
	Meta         CollectionMeta
	Arrows       []CollectionArrow
}

// CollectionArrowEntry is the raw translator output before namespace derivation.
// Exactly one of Path or Namespace must be set. AUID is optional and only
// valid alongside Path — it overrides the identity that otherwise derives
// from Path's last segment.
type CollectionArrowEntry struct {
	Path      string `yaml:"path"`
	Namespace string `yaml:"namespace"`
	AUID      string `yaml:"auid"`
}

// CollectionArrow is a resolved arrow reference with its final namespace.
// SourcePath is the arrow's location inside the collection's own repository
// (empty for an external arrow, which carries no path of its own).
type CollectionArrow struct {
	Namespace  Namespace
	IsLocal    bool
	SourcePath string
}
