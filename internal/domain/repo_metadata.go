package domain

// RepoMetadata is what a git host says about a repository beyond its files:
// the description its maintainers wrote and the avatar of its owner.
type RepoMetadata struct {
	Description string
	AvatarURL   string
}
