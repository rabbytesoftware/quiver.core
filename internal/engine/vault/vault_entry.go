package vault

import "time"

type vaultEntry[T any] struct {
	CachedAt time.Time `json:"cached_at"`
	OS       string    `json:"os"`
	Manifest T         `json:"manifest"`
}
