// Package config manages the CLI's context store at ~/.quiver/cli.yaml.
// A context names a quiver.core instance (local Unix socket or remote TCP).
// The daemon's own config lives in ~/.quiver/config.yaml and is not touched
// by this package.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// LocalContextName is the auto-created context pointing at the local socket.
const LocalContextName = "local"

// Context is one named quiver.core instance.
type Context struct {
	Name     string `yaml:"name" json:"name"`
	Server   string `yaml:"server" json:"server"`
	Token    string `yaml:"token,omitempty" json:"token,omitempty"`
	Insecure bool   `yaml:"insecure,omitempty" json:"insecure,omitempty"`
}

// Config is the loaded context store. Mutations persist immediately.
type Config struct {
	path string
	file fileModel
}

type fileModel struct {
	ActiveContext string    `yaml:"active_context" json:"active_context"`
	Contexts      []Context `yaml:"contexts" json:"contexts"`
}

// DefaultPath returns ~/.quiver/cli.yaml.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("config: resolve home dir: %w", err)
	}
	return filepath.Join(home, ".quiver", "cli.yaml"), nil
}

// DefaultLocalServer returns the default daemon socket URI.
func DefaultLocalServer() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "unix:///tmp/quiver.sock"
	}
	return "unix://" + filepath.Join(home, ".quiver", "quiver.sock")
}

// Load reads the context store, creating the in-memory default when the file
// does not exist yet. The file is only written on first mutation.
func Load(path string) (*Config, error) {
	cfg := &Config{path: path}

	raw, err := os.ReadFile(path) //nolint:gosec // path is the user's own config file
	if os.IsNotExist(err) {
		cfg.file = defaultFile()
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	if err := yaml.Unmarshal(raw, &cfg.file); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}
	if len(cfg.file.Contexts) == 0 {
		cfg.file = defaultFile()
	}
	return cfg, nil
}

func defaultFile() fileModel {
	return fileModel{
		ActiveContext: LocalContextName,
		Contexts: []Context{
			{Name: LocalContextName, Server: DefaultLocalServer()},
		},
	}
}

// List returns all contexts.
func (c *Config) List() []Context {
	return c.file.Contexts
}

// ActiveName returns the active context's name.
func (c *Config) ActiveName() string {
	return c.file.ActiveContext
}

// Active returns the active context.
func (c *Config) Active() (Context, error) {
	return c.Get(c.file.ActiveContext)
}

// Get returns a context by name.
func (c *Config) Get(name string) (Context, error) {
	for _, ctx := range c.file.Contexts {
		if ctx.Name == name {
			return ctx, nil
		}
	}
	return Context{}, fmt.Errorf("config: context %q not found", name)
}

// Add registers a context. use activates it immediately.
func (c *Config) Add(ctx Context, use bool) error {
	if ctx.Name == "" {
		return fmt.Errorf("config: context name is empty")
	}
	if ctx.Server == "" {
		return fmt.Errorf("config: context %q: server is required", ctx.Name)
	}
	if _, err := c.Get(ctx.Name); err == nil {
		return fmt.Errorf("config: context %q already exists", ctx.Name)
	}

	c.file.Contexts = append(c.file.Contexts, ctx)
	if use {
		c.file.ActiveContext = ctx.Name
	}
	return c.save()
}

// Use switches the active context.
func (c *Config) Use(name string) error {
	if _, err := c.Get(name); err != nil {
		return err
	}
	c.file.ActiveContext = name
	return c.save()
}

// Remove deletes a context. Removing the active context requires force; the
// first remaining context becomes active.
func (c *Config) Remove(name string, force bool) error {
	if _, err := c.Get(name); err != nil {
		return err
	}
	if name == c.file.ActiveContext && !force {
		return fmt.Errorf("config: %q is the active context, use --force or switch first", name)
	}

	kept := make([]Context, 0, len(c.file.Contexts))
	for _, ctx := range c.file.Contexts {
		if ctx.Name != name {
			kept = append(kept, ctx)
		}
	}
	if len(kept) == 0 {
		kept = defaultFile().Contexts
	}
	c.file.Contexts = kept
	if name == c.file.ActiveContext {
		c.file.ActiveContext = kept[0].Name
	}
	return c.save()
}

// Resolve picks the server URI for a command invocation.
// Precedence: explicit --server flag > named --context > active context.
func (c *Config) Resolve(flagServer, flagContext string) (string, error) {
	if flagServer != "" {
		return flagServer, nil
	}
	if flagContext != "" {
		ctx, err := c.Get(flagContext)
		if err != nil {
			return "", err
		}
		return ctx.Server, nil
	}
	ctx, err := c.Active()
	if err != nil {
		return "", err
	}
	return ctx.Server, nil
}

func (c *Config) save() error {
	if err := os.MkdirAll(filepath.Dir(c.path), 0o750); err != nil {
		return fmt.Errorf("config: create dir for %s: %w", c.path, err)
	}
	raw, err := yaml.Marshal(c.file)
	if err != nil {
		return fmt.Errorf("config: marshal: %w", err)
	}
	if err := os.WriteFile(c.path, raw, 0o600); err != nil {
		return fmt.Errorf("config: write %s: %w", c.path, err)
	}
	return nil
}
