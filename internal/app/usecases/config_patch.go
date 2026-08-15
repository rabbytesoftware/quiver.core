package usecases

// ConfigPatch is a sparse configuration change. Every field is optional: an
// absent field is left alone, a null field is restored to its default, and a
// field carrying a value is set to it.
//
// Each field is documented as its underlying scalar rather than as the
// Optional wrapper, because that is what goes on the wire.
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
	Enabled            Optional[bool] `json:"enabled" swaggertype:"boolean" extensions:"x-nullable"`
	EphemeralPortStart Optional[int]  `json:"ephemeral_port_start" swaggertype:"integer" extensions:"x-nullable"`
	EphemeralPortEnd   Optional[int]  `json:"ephemeral_port_end" swaggertype:"integer" extensions:"x-nullable"`
}

// APIPatch is the api section of a ConfigPatch.
type APIPatch struct {
	Host Optional[string] `json:"host" swaggertype:"string" extensions:"x-nullable"`
}

// LoggerPatch is the logger section of a ConfigPatch.
type LoggerPatch struct {
	Enabled Optional[bool]   `json:"enabled" swaggertype:"boolean" extensions:"x-nullable"`
	Level   Optional[string] `json:"level" swaggertype:"string" extensions:"x-nullable"`
}

// ManifoldPatch is the manifold section of a ConfigPatch.
type ManifoldPatch struct {
	FetchTimeout Optional[string] `json:"fetch_timeout" swaggertype:"string" extensions:"x-nullable"`
}

// VaultPatch is the vault section of a ConfigPatch.
type VaultPatch struct {
	SweepInterval Optional[string] `json:"sweep_interval" swaggertype:"string" extensions:"x-nullable"`
	TTL           Optional[string] `json:"ttl" swaggertype:"string" extensions:"x-nullable"`
	IndexTTL      Optional[string] `json:"index_ttl" swaggertype:"string" extensions:"x-nullable"`
}

// ArrowsPatch is the arrows section of a ConfigPatch.
type ArrowsPatch struct {
	AutoRetry AutoRetryPatch `json:"auto_retry"`
}

// AutoRetryPatch is the arrows.auto_retry subsection of a ConfigPatch.
type AutoRetryPatch struct {
	Enabled Optional[bool] `json:"enabled" swaggertype:"boolean" extensions:"x-nullable"`
	Retries Optional[int]  `json:"retries" swaggertype:"integer" extensions:"x-nullable"`
}

// SearchPatch is the search section of a ConfigPatch.
type SearchPatch struct {
	PerProviderLimit Optional[int]    `json:"per_provider_limit" swaggertype:"integer" extensions:"x-nullable"`
	FetchConcurrency Optional[int]    `json:"fetch_concurrency" swaggertype:"integer" extensions:"x-nullable"`
	ProviderTimeout  Optional[string] `json:"provider_timeout" swaggertype:"string" extensions:"x-nullable"`
}
