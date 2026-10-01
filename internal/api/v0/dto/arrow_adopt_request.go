package dto

// AdoptRequestDTO declares the ref an arrow is already installed at.
type AdoptRequestDTO struct {
	// ResolvedRef is the tag or branch the caller runs. It must be one the
	// namespace's selector could resolve to: a member of the channel, a tag
	// the constraint matches, the pin's own ref, or a ref at the commit.
	ResolvedRef string `json:"resolved_ref" yaml:"resolved_ref" example:"v1.2.0"`
}
