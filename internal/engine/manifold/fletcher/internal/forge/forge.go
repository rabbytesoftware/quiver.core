package forge

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/picker"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/readme"
)

const (
	schemaV0   = "arrow@v0"
	yamlIndent = 2
	fenceOpen  = "\n\n```arrow\n"
	fenceClose = "```\n"
)

type Input struct {
	Repo        string
	Name        string
	Description string
	URL         string
	Media       domain.ArrowMedia
	Readme      string
	Generator   domain.ArrowGenerator
	Picks       map[domain.OS]picker.Pick
}

func Render(
	in Input,
) ([]byte, error) {
	var body bytes.Buffer
	enc := yaml.NewEncoder(&body)
	enc.SetIndent(yamlIndent)
	if err := enc.Encode(newDocument(in)); err != nil {
		return nil, fmt.Errorf("forge: encode manifest: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("forge: close manifest encoder: %w", err)
	}
	return []byte(prose(in) + fenceOpen + body.String() + fenceClose), nil
}

func prose(
	in Input,
) string {
	return readme.NeutralizeFences(proseText(in))
}

func proseText(
	in Input,
) string {
	if text := strings.TrimSpace(in.Readme); text != "" {
		return text
	}
	if text := singleLine(in.Description); text != "" {
		return text
	}
	return singleLine(in.Name)
}

func singleLine(
	text string,
) string {
	return strings.Join(strings.Fields(text), " ")
}
