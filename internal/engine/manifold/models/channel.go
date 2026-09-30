package models

import "errors"

// ChannelInfo describes one channel a namespace's repository publishes.
type ChannelInfo struct {
	// Name is the channel's identity: a classified name (§4.1) for an
	// ordered channel, or the literal ref name for a pointer channel.
	Name string
	// Kind is "ordered" or "pointer".
	Kind string
	// Latest is the highest-precedence tag for an ordered channel, or the
	// literal ref (tag or branch name) for a pointer channel.
	Latest string
	// Count is the number of tags classified into this channel. Always 0
	// for a pointer channel, which by definition has exactly one member.
	Count int
	// Members lists every tag in this channel, ordered by precedence
	// (highest first) — Members[0] always equals Latest. Empty for a
	// pointer channel, which by definition has exactly one member: itself.
	Members []string
	// IsDefaultBranchFallback is true only for the synthetic entry
	// ListChannels appends when a repository has no tags at all: the
	// default branch offered as something to show in a channel picker,
	// not a published channel.
	IsDefaultBranchFallback bool
}

// ErrNoLatestStable reports that a repository publishes no stable release.
var ErrNoLatestStable = errors.New("manifold: no latest stable release")

// ErrNoTagInChannel reports that a repository has no tag classified into
// the requested channel.
var ErrNoTagInChannel = errors.New("manifold: no tag in channel")
