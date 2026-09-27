package forge

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/readme"
)

const (
	schemaV0   = "arrow@v0"
	yamlIndent = 2
	fenceOpen  = "\n\n```arrow\n"
	fenceClose = "```\n"
)

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

func newDocument(
	in Input,
) document {
	name := exposeName(in.Repo)
	targets := make(map[string]target, len(in.Picks))
	for platform, pick := range in.Picks {
		if file, ok := LocalFile(pick); ok {
			targets[string(platform)] = newTarget(name, file, platform, pick)
		}
	}
	return document{
		Schema: schemaV0,
		Metadata: metadata{
			Name:        singleLine(in.Name),
			Description: singleLine(in.Description),
			URL:         singleLine(in.URL),
			Media:       media{Icon: singleLine(in.Media.Icon), Banner: singleLine(in.Media.Banner)},
			Generator: generator{
				Name:       in.Generator.Name,
				Confidence: in.Generator.Confidence,
				Warnings:   in.Generator.Warnings,
			},
		},
		Targets: targets,
	}
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
