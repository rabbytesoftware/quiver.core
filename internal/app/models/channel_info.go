package models

// ChannelInfo describes one release channel a namespace's repository
// publishes. It is the app-layer boundary type for engine/manifold's
// ChannelInfo — the API layer converts from this, never from manifold's
// type directly, matching how every other DTO in this codebase only
// imports internal/domain and internal/app/models, never internal/engine/*.
type ChannelInfo struct {
	Name   string
	Kind   string
	Latest string
	Count  int
	// Members lists every tag in this channel, ordered by precedence
	// (highest first) — Members[0] always equals Latest. Empty for a
	// pointer channel.
	Members []string
}
