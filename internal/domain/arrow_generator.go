package domain

const (
	ArrowOriginDeclared = "declared"
	ArrowOriginInferred = "inferred"
)

type ArrowGenerator struct {
	Name       string   `yaml:"name"       json:"name"`
	Confidence string   `yaml:"confidence" json:"confidence"`
	Warnings   []string `yaml:"warnings"   json:"warnings,omitempty"`
}
