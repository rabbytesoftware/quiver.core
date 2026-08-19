package config

import (
	"context"
	"fmt"
	"path/filepath"

	yaml "gopkg.in/yaml.v3"

	"github.com/rabbytesoftware/quiver.core/internal/core/fns"
	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
)

// Configured reads the user's configuration file fresh from disk, merges it
// onto the compiled-in defaults and sanitizes the result. It reports what the
// next daemon start will use, which is not necessarily what the running
// process holds. A missing file yields the defaults and no error.
func Configured() (ConfigData, []FieldError, error) {
	return configuredAt(filepath.Clean(metadata.GetConfigPath()))
}

// Save writes the user's configuration file so that the next daemon start
// resolves to data. Only fields differing from the compiled-in defaults are
// written, so a field restored to its default disappears from the file.
func Save(
	data ConfigData,
) error {
	return saveAt(filepath.Clean(metadata.GetConfigPath()), data)
}

func configuredAt(
	path string,
) (ConfigData, []FieldError, error) {
	merged := getDefaultConfig()

	raw, err := fns.Read(context.Background(), path)
	if err != nil {
		return merged.Config, nil, nil
	}

	if err := yaml.Unmarshal(raw, merged); err != nil {
		return ConfigData{}, nil, fmt.Errorf("config: parse %s: %w", path, err)
	}

	corrected := Sanitize(&merged.Config)

	return merged.Config, corrected, nil
}

func saveAt(
	path string,
	data ConfigData,
) error {
	node, err := buildOverlay(data, Defaults())
	if err != nil {
		return err
	}

	raw, err := yaml.Marshal(node)
	if err != nil {
		return fmt.Errorf("config: encode overlay: %w", err)
	}

	ctx := context.Background()
	tmp := path + ".tmp"

	if err := fns.Write(ctx, tmp, raw); err != nil {
		return fmt.Errorf("config: write overlay: %w", err)
	}

	if err := fns.Rename(ctx, tmp, path); err != nil {
		return fmt.Errorf("config: replace overlay: %w", err)
	}

	return nil
}

// buildOverlay renders data as YAML containing only the leaves that differ
// from def. It marshals the whole document and prunes it, so the encoder
// decides each scalar's type from the real Go value and a new setting needs no
// code here.
func buildOverlay(
	data ConfigData,
	def ConfigData,
) (*yaml.Node, error) {
	raw, err := yaml.Marshal(Config{Config: data})
	if err != nil {
		return nil, fmt.Errorf("config: marshal overlay: %w", err)
	}

	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("config: reparse overlay: %w", err)
	}

	keep := make(map[string]bool)
	for _, key := range Differing(def, data) {
		keep["config."+key] = true
	}

	if len(root.Content) == 1 {
		prune(root.Content[0], "", keep)
	}

	return &root, nil
}

// prune drops every mapping entry that leads to no kept leaf.
func prune(
	node *yaml.Node,
	prefix string,
	keep map[string]bool,
) {
	if node.Kind != yaml.MappingNode {
		return
	}

	kept := make([]*yaml.Node, 0, len(node.Content))

	for i := 0; i+1 < len(node.Content); i += 2 {
		name, value := node.Content[i], node.Content[i+1]

		path := name.Value
		if prefix != "" {
			path = prefix + "." + name.Value
		}

		if value.Kind == yaml.MappingNode {
			prune(value, path, keep)
			if len(value.Content) > 0 {
				kept = append(kept, name, value)
			}
			continue
		}

		if keep[path] {
			kept = append(kept, name, value)
		}
	}

	node.Content = kept
}
