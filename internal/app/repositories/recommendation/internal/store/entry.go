package store

// Entry is one ranked arrow of a snapshot. The store keeps what the ranking
// knew; everything else about the arrow is read from the vault when the shelf
// is shown.
type Entry struct {
	Namespace string
	Stars     int
	Source    string
}
