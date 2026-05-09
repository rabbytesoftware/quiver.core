package domain

type Credit struct {
	Name  string `yaml:"name"           json:"name"`
	Email string `yaml:"email,omitempty" json:"email,omitempty"`
	URL   string `yaml:"url,omitempty"   json:"url,omitempty"`
}
