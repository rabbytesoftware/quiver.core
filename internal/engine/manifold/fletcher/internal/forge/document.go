package forge

import "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/picker"

type document struct {
	Schema   string            `yaml:"schema"`
	Metadata metadata          `yaml:"metadata"`
	Targets  map[string]target `yaml:"targets"`
}

type metadata struct {
	Name        string    `yaml:"name"`
	Description string    `yaml:"description,omitempty"`
	URL         string    `yaml:"url,omitempty"`
	Media       media     `yaml:"media,omitempty"`
	Generator   generator `yaml:"generator"`
}

type generator struct {
	Name       string   `yaml:"name"`
	Confidence string   `yaml:"confidence"`
	Warnings   []string `yaml:"warnings,omitempty"`
}

type media struct {
	Icon   string `yaml:"icon,omitempty"`
	Banner string `yaml:"banner,omitempty"`
}

func newDocument(
	in Input,
) document {
	name := exposeName(in.Repo)
	targets := make(map[string]target, len(in.Picks))
	for platform, pick := range in.Picks {
		if file, ok := LocalFile(pick); ok {
			targets[string(platform)] = newTarget(name, file, platform, checksummed(pick, in.Unpinned))
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

func checksummed(
	pick picker.Pick,
	unpinned bool,
) picker.Pick {
	if unpinned {
		pick.Asset.Digest = ""
	}
	return pick
}
