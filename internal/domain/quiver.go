package domain

type Quiver struct {
	ID              string      `json:"id"`
	Name            string      `json:"name"`
	Description     string      `json:"description"`
	Banner          URL         `json:"banner"`
	URL             URL         `json:"url"`
	Security        Security    `json:"security"`
	Maintainers     []string    `json:"maintainers"`
	Version         string      `json:"version"`
	InstalledArrows []Arrow     `json:"installed_arrows"`
	ListedArrows    []Namespace `json:"listed_arrows"`
}
