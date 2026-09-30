package recommendation

// SourceConfig is one query-less search a shelf is built from, as written in
// the configuration file.
type SourceConfig struct {
	Host         string
	Sort         string
	MinStars     int
	MaxStars     int
	PushedWithin string
}

// ShelfConfig is one shelf as written in the configuration file.
type ShelfConfig struct {
	ID      string
	Title   string
	Limit   int
	Sources []SourceConfig
}

// Config holds the values read once at construction. Durations are the strings
// the configuration file holds; PushedWithin also accepts a day count such as
// 90d. A shelf that does not validate is skipped with a warning rather than
// refusing to start the daemon over an optional feature.
type Config struct {
	Enabled         bool
	RefreshInterval string
	CandidateBudget int
	MinEntries      int
	Shelves         []ShelfConfig
}
