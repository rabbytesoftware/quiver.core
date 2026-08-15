package config

import (
	"context"
	"fmt"
	"path/filepath"

	yaml "gopkg.in/yaml.v3"

	"github.com/rabbytesoftware/quiver.core/internal/core/fns"
	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
)

type overlayFile struct {
	Config overlayData `yaml:"config"`
}

type overlayData struct {
	Netbridge *overlayNetbridge `yaml:"netbridge,omitempty"`
	API       *overlayAPI       `yaml:"api,omitempty"`
	Logger    *overlayLogger    `yaml:"logger,omitempty"`
	Manifold  *overlayManifold  `yaml:"manifold,omitempty"`
	Vault     *overlayVault     `yaml:"vault,omitempty"`
	Arrows    *overlayArrows    `yaml:"arrows,omitempty"`
	Search    *overlaySearch    `yaml:"search,omitempty"`
}

type overlayNetbridge struct {
	Enabled            *bool `yaml:"enabled,omitempty"`
	EphemeralPortStart *int  `yaml:"ephemeral_port_start,omitempty"`
	EphemeralPortEnd   *int  `yaml:"ephemeral_port_end,omitempty"`
}

type overlayAPI struct {
	Host *string `yaml:"host,omitempty"`
}

type overlayLogger struct {
	Enabled *bool   `yaml:"enabled,omitempty"`
	Level   *string `yaml:"level,omitempty"`
}

type overlayManifold struct {
	FetchTimeout *string `yaml:"fetch_timeout,omitempty"`
}

type overlayVault struct {
	SweepInterval *string `yaml:"sweep_interval,omitempty"`
	TTL           *string `yaml:"ttl,omitempty"`
	IndexTTL      *string `yaml:"index_ttl,omitempty"`
}

type overlayArrows struct {
	AutoRetry *overlayAutoRetry `yaml:"auto_retry,omitempty"`
}

type overlayAutoRetry struct {
	Enabled *bool `yaml:"enabled,omitempty"`
	Retries *int  `yaml:"retries,omitempty"`
}

type overlaySearch struct {
	PerProviderLimit *int    `yaml:"per_provider_limit,omitempty"`
	FetchConcurrency *int    `yaml:"fetch_concurrency,omitempty"`
	ProviderTimeout  *string `yaml:"provider_timeout,omitempty"`
}

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
	raw, err := yaml.Marshal(overlayFile{Config: buildOverlay(data, Defaults())})
	if err != nil {
		return fmt.Errorf("config: marshal overlay: %w", err)
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

func buildOverlay(
	data ConfigData,
	def ConfigData,
) overlayData {
	return overlayData{
		Netbridge: buildNetbridgeOverlay(data.Netbridge, def.Netbridge),
		API:       buildAPIOverlay(data.API, def.API),
		Logger:    buildLoggerOverlay(data.Logger, def.Logger),
		Manifold:  buildManifoldOverlay(data.Manifold, def.Manifold),
		Vault:     buildVaultOverlay(data.Vault, def.Vault),
		Arrows:    buildArrowsOverlay(data.Arrows, def.Arrows),
		Search:    buildSearchOverlay(data.Search, def.Search),
	}
}

func buildNetbridgeOverlay(
	cur Netbridge,
	def Netbridge,
) *overlayNetbridge {
	o := overlayNetbridge{
		Enabled:            diffBool(cur.Enabled, def.Enabled),
		EphemeralPortStart: diffInt(cur.EphemeralPortStart, def.EphemeralPortStart),
		EphemeralPortEnd:   diffInt(cur.EphemeralPortEnd, def.EphemeralPortEnd),
	}

	if o.Enabled == nil && o.EphemeralPortStart == nil && o.EphemeralPortEnd == nil {
		return nil
	}

	return &o
}

func buildAPIOverlay(
	cur API,
	def API,
) *overlayAPI {
	host := diffStr(cur.Host, def.Host)
	if host == nil {
		return nil
	}

	return &overlayAPI{Host: host}
}

func buildLoggerOverlay(
	cur Logger,
	def Logger,
) *overlayLogger {
	o := overlayLogger{
		Enabled: diffBool(cur.Enabled, def.Enabled),
		Level:   diffStr(cur.Level, def.Level),
	}

	if o.Enabled == nil && o.Level == nil {
		return nil
	}

	return &o
}

func buildManifoldOverlay(
	cur Manifold,
	def Manifold,
) *overlayManifold {
	timeout := diffStr(cur.FetchTimeout, def.FetchTimeout)
	if timeout == nil {
		return nil
	}

	return &overlayManifold{FetchTimeout: timeout}
}

func buildVaultOverlay(
	cur Vault,
	def Vault,
) *overlayVault {
	o := overlayVault{
		SweepInterval: diffStr(cur.SweepInterval, def.SweepInterval),
		TTL:           diffStr(cur.TTL, def.TTL),
		IndexTTL:      diffStr(cur.IndexTTL, def.IndexTTL),
	}

	if o.SweepInterval == nil && o.TTL == nil && o.IndexTTL == nil {
		return nil
	}

	return &o
}

func buildArrowsOverlay(
	cur Arrows,
	def Arrows,
) *overlayArrows {
	o := overlayAutoRetry{
		Enabled: diffBool(cur.AutoRetry.Enabled, def.AutoRetry.Enabled),
		Retries: diffInt(cur.AutoRetry.Retries, def.AutoRetry.Retries),
	}

	if o.Enabled == nil && o.Retries == nil {
		return nil
	}

	return &overlayArrows{AutoRetry: &o}
}

func buildSearchOverlay(
	cur Search,
	def Search,
) *overlaySearch {
	o := overlaySearch{
		PerProviderLimit: diffInt(cur.PerProviderLimit, def.PerProviderLimit),
		FetchConcurrency: diffInt(cur.FetchConcurrency, def.FetchConcurrency),
		ProviderTimeout:  diffStr(cur.ProviderTimeout, def.ProviderTimeout),
	}

	if o.PerProviderLimit == nil && o.FetchConcurrency == nil && o.ProviderTimeout == nil {
		return nil
	}

	return &o
}

func diffBool(
	cur bool,
	def bool,
) *bool {
	if cur == def {
		return nil
	}

	return &cur
}

func diffInt(
	cur int,
	def int,
) *int {
	if cur == def {
		return nil
	}

	return &cur
}

func diffStr(
	cur string,
	def string,
) *string {
	if cur == def {
		return nil
	}

	return &cur
}
