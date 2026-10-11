package forge

import (
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/picker"
)

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
	addStartStop(name, targets)
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

// addStartStop gives every target a start and a stop when any of them exposes
// an app, since a manifest cannot mix targets with and without an execute. A
// target that placed no app still validates, and its start fails saying so.
func addStartStop(
	name string,
	targets map[string]target,
) {
	hasApp := false
	for _, t := range targets {
		hasApp = hasApp || len(t.Expose.Desktop) > 0
	}
	if !hasApp {
		return
	}
	for key, t := range targets {
		t.Lifecycle.Execute, t.Lifecycle.Stop = startStopSteps(name, domain.OS(key).IsWindows())
		targets[key] = t
	}
}
