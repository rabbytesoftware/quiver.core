package usecases

// ConfigPatch is a sparse configuration change. Every field is optional: an
// absent field is left alone, a null field is restored to its default, and a
// field carrying a value is set to it.
type ConfigPatch struct {
	Netbridge NetbridgePatch `json:"netbridge"`
	API       APIPatch       `json:"api"`
	Logger    LoggerPatch    `json:"logger"`
	Manifold  ManifoldPatch  `json:"manifold"`
	Vault     VaultPatch     `json:"vault"`
	Arrows    ArrowsPatch    `json:"arrows"`
	Search    SearchPatch    `json:"search"`
}

// NetbridgePatch is the netbridge section of a ConfigPatch.
type NetbridgePatch struct {
	Enabled            Optional[bool] `json:"enabled"`
	EphemeralPortStart Optional[int]  `json:"ephemeral_port_start"`
	EphemeralPortEnd   Optional[int]  `json:"ephemeral_port_end"`
}

// APIPatch is the api section of a ConfigPatch.
type APIPatch struct {
	Host Optional[string] `json:"host"`
}

// LoggerPatch is the logger section of a ConfigPatch.
type LoggerPatch struct {
	Enabled Optional[bool]   `json:"enabled"`
	Level   Optional[string] `json:"level"`
}

// ManifoldPatch is the manifold section of a ConfigPatch.
type ManifoldPatch struct {
	FetchTimeout Optional[string] `json:"fetch_timeout"`
}

// VaultPatch is the vault section of a ConfigPatch.
type VaultPatch struct {
	SweepInterval Optional[string] `json:"sweep_interval"`
	TTL           Optional[string] `json:"ttl"`
	IndexTTL      Optional[string] `json:"index_ttl"`
}

// ArrowsPatch is the arrows section of a ConfigPatch.
type ArrowsPatch struct {
	AutoRetry AutoRetryPatch `json:"auto_retry"`
}

// AutoRetryPatch is the arrows.auto_retry subsection of a ConfigPatch.
type AutoRetryPatch struct {
	Enabled Optional[bool] `json:"enabled"`
	Retries Optional[int]  `json:"retries"`
}

// SearchPatch is the search section of a ConfigPatch.
type SearchPatch struct {
	PerProviderLimit Optional[int]    `json:"per_provider_limit"`
	FetchConcurrency Optional[int]    `json:"fetch_concurrency"`
	ProviderTimeout  Optional[string] `json:"provider_timeout"`
}
