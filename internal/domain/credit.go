package domain

// Credit represents a contributor to an Arrow, with optional contact details.
type Credit struct {
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
	URL   string `json:"url,omitempty"`
}
