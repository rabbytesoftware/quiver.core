package fletcher

type Hint struct {
	Name        string
	Description string
}

func (h Hint) nameOr(
	repo string,
) string {
	if h.Name == "" {
		return repo
	}
	return h.Name
}
