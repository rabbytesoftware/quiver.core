package shelf

type candidate struct {
	name     string
	display  string
	target   string
	icon     string
	depth    int
	declared bool
}

func (c candidate) displayName() string {
	if c.display != "" {
		return c.display
	}
	return c.name
}
