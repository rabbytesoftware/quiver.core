package dto

import "github.com/rabbytesoftware/quiver.core/internal/app/usecases"

// ConfigDTO is the daemon configuration seen three ways at once.
//
// Running is what the process is using, Configured is what the next start will
// use, and Defaults is what ships in the binary. RestartRequired names the
// fields where Running and Configured disagree.
type ConfigDTO struct {
	Running         RunningConfigDTO `json:"running"`
	Configured      ConfigSectionDTO `json:"configured"`
	Defaults        ConfigSectionDTO `json:"defaults"`
	RestartRequired []string         `json:"restart_required"`
}

// ConfigSectionDTO is a complete configuration document.
type ConfigSectionDTO struct {
	Netbridge NetbridgeDTO `json:"netbridge"`
	API       APIConfigDTO `json:"api"`
	Logger    LoggerDTO    `json:"logger"`
	Manifold  ManifoldDTO  `json:"manifold"`
	Vault     VaultDTO     `json:"vault"`
	Arrows    ArrowsDTO    `json:"arrows"`
	Search    SearchDTO    `json:"search"`
}

// RunningConfigDTO is the configuration in force. It carries no api section:
// the --host flag can override the configured host at start, so the daemon
// cannot report a bind address from configuration alone.
type RunningConfigDTO struct {
	Netbridge NetbridgeDTO `json:"netbridge"`
	Logger    LoggerDTO    `json:"logger"`
	Manifold  ManifoldDTO  `json:"manifold"`
	Vault     VaultDTO     `json:"vault"`
	Arrows    ArrowsDTO    `json:"arrows"`
	Search    SearchDTO    `json:"search"`
}

// NetbridgeDTO is the netbridge configuration section.
type NetbridgeDTO struct {
	Enabled            bool `json:"enabled"`
	EphemeralPortStart int  `json:"ephemeral_port_start"`
	EphemeralPortEnd   int  `json:"ephemeral_port_end"`
}

// APIConfigDTO is the api configuration section.
type APIConfigDTO struct {
	Host string `json:"host"`
}

// LoggerDTO is the logger configuration section.
type LoggerDTO struct {
	Enabled bool   `json:"enabled"`
	Level   string `json:"level"`
}

// ManifoldDTO is the manifold configuration section.
type ManifoldDTO struct {
	FetchTimeout string `json:"fetch_timeout"`
}

// VaultDTO is the vault configuration section.
type VaultDTO struct {
	SweepInterval string `json:"sweep_interval"`
	TTL           string `json:"ttl"`
	IndexTTL      string `json:"index_ttl"`
}

// ArrowsDTO is the arrows configuration section.
type ArrowsDTO struct {
	AutoRetry AutoRetryDTO `json:"auto_retry"`
}

// AutoRetryDTO is the arrows.auto_retry configuration subsection.
type AutoRetryDTO struct {
	Enabled bool `json:"enabled"`
	Retries int  `json:"retries"`
}

// SearchDTO is the search configuration section.
type SearchDTO struct {
	PerProviderLimit int    `json:"per_provider_limit"`
	FetchConcurrency int    `json:"fetch_concurrency"`
	ProviderTimeout  string `json:"provider_timeout"`
}

// ConfigDTOFrom maps a usecase configuration view onto the wire shape.
func ConfigDTOFrom(
	view usecases.ConfigView,
) ConfigDTO {
	restart := view.RestartRequired
	if restart == nil {
		restart = []string{}
	}

	return ConfigDTO{
		Running:         runningFrom(view),
		Configured:      configuredFrom(view),
		Defaults:        defaultsFrom(view),
		RestartRequired: restart,
	}
}

func runningFrom(
	view usecases.ConfigView,
) RunningConfigDTO {
	d := view.Running

	return RunningConfigDTO{
		Netbridge: NetbridgeDTO{
			Enabled:            d.Netbridge.Enabled,
			EphemeralPortStart: d.Netbridge.EphemeralPortStart,
			EphemeralPortEnd:   d.Netbridge.EphemeralPortEnd,
		},
		Logger:   LoggerDTO{Enabled: d.Logger.Enabled, Level: d.Logger.Level},
		Manifold: ManifoldDTO{FetchTimeout: d.Manifold.FetchTimeout},
		Vault: VaultDTO{
			SweepInterval: d.Vault.SweepInterval,
			TTL:           d.Vault.TTL,
			IndexTTL:      d.Vault.IndexTTL,
		},
		Arrows: ArrowsDTO{AutoRetry: AutoRetryDTO{
			Enabled: d.Arrows.AutoRetry.Enabled,
			Retries: d.Arrows.AutoRetry.Retries,
		}},
		Search: SearchDTO{
			PerProviderLimit: d.Search.PerProviderLimit,
			FetchConcurrency: d.Search.FetchConcurrency,
			ProviderTimeout:  d.Search.ProviderTimeout,
		},
	}
}

func configuredFrom(
	view usecases.ConfigView,
) ConfigSectionDTO {
	d := view.Configured

	return ConfigSectionDTO{
		Netbridge: NetbridgeDTO{
			Enabled:            d.Netbridge.Enabled,
			EphemeralPortStart: d.Netbridge.EphemeralPortStart,
			EphemeralPortEnd:   d.Netbridge.EphemeralPortEnd,
		},
		API:      APIConfigDTO{Host: d.API.Host},
		Logger:   LoggerDTO{Enabled: d.Logger.Enabled, Level: d.Logger.Level},
		Manifold: ManifoldDTO{FetchTimeout: d.Manifold.FetchTimeout},
		Vault: VaultDTO{
			SweepInterval: d.Vault.SweepInterval,
			TTL:           d.Vault.TTL,
			IndexTTL:      d.Vault.IndexTTL,
		},
		Arrows: ArrowsDTO{AutoRetry: AutoRetryDTO{
			Enabled: d.Arrows.AutoRetry.Enabled,
			Retries: d.Arrows.AutoRetry.Retries,
		}},
		Search: SearchDTO{
			PerProviderLimit: d.Search.PerProviderLimit,
			FetchConcurrency: d.Search.FetchConcurrency,
			ProviderTimeout:  d.Search.ProviderTimeout,
		},
	}
}

func defaultsFrom(
	view usecases.ConfigView,
) ConfigSectionDTO {
	d := view.Defaults

	return ConfigSectionDTO{
		Netbridge: NetbridgeDTO{
			Enabled:            d.Netbridge.Enabled,
			EphemeralPortStart: d.Netbridge.EphemeralPortStart,
			EphemeralPortEnd:   d.Netbridge.EphemeralPortEnd,
		},
		API:      APIConfigDTO{Host: d.API.Host},
		Logger:   LoggerDTO{Enabled: d.Logger.Enabled, Level: d.Logger.Level},
		Manifold: ManifoldDTO{FetchTimeout: d.Manifold.FetchTimeout},
		Vault: VaultDTO{
			SweepInterval: d.Vault.SweepInterval,
			TTL:           d.Vault.TTL,
			IndexTTL:      d.Vault.IndexTTL,
		},
		Arrows: ArrowsDTO{AutoRetry: AutoRetryDTO{
			Enabled: d.Arrows.AutoRetry.Enabled,
			Retries: d.Arrows.AutoRetry.Retries,
		}},
		Search: SearchDTO{
			PerProviderLimit: d.Search.PerProviderLimit,
			FetchConcurrency: d.Search.FetchConcurrency,
			ProviderTimeout:  d.Search.ProviderTimeout,
		},
	}
}
