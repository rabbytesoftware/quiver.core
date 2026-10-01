package providers

// bitbucketProvider answers for a Bitbucket host. Bitbucket serves raw files
// and names default branches like any other host, so its manifests are
// fetchable; it exposes no repository search Quiver can query, so it
// contributes no candidates: a host with nothing host-specific left to say is
// the plain host and nothing more.
type bitbucketProvider struct {
	host
}

// NewBitbucket builds the provider answering for a Bitbucket host.
func NewBitbucket(
	cfg Config,
) Provider {
	return &bitbucketProvider{host: newHost(cfg)}
}
