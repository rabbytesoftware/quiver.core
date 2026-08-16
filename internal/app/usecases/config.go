package usecases

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	repoconfig "github.com/rabbytesoftware/quiver.core/internal/app/repositories/config"
)

// Config is a complete daemon configuration document.
type Config = repoconfig.Data

// ConfigView is the daemon configuration seen three ways at once: what the
// process is running with, what the next start will use, and what ships in the
// binary. A client needs all three to show a current value, offer a reset, and
// say honestly whether a change has taken effect.
type ConfigView struct {
	Running         repoconfig.Data
	Configured      repoconfig.Data
	Defaults        repoconfig.Data
	RestartRequired []string
}

// PatchResult reports which fields a patch persisted and which it refused.
type PatchResult struct {
	Applied  []string
	Rejected []repoconfig.FieldError
}

const (
	keyHost      = "api.host"
	keyPortStart = "netbridge.ephemeral_port_start"
	keyPortEnd   = "netbridge.ephemeral_port_end"
)

// ConfigUsecase reads and edits the daemon configuration. Every change takes
// effect on the next daemon start; nothing here mutates the running process.
type ConfigUsecase interface {
	// Get returns the running, configured and default configurations together
	// with the fields whose change is still waiting on a restart.
	Get(ctx context.Context) (ConfigView, error)

	// Patch persists the settings the body names and reports the ones it
	// refused. The body is a sparse configuration document: an absent field is
	// left alone, a null field is restored to its default, and a field
	// carrying a value is set to it. It returns ErrInvalidConfig when nothing
	// at all could be applied.
	Patch(ctx context.Context, body json.RawMessage) (PatchResult, error)
}

type configUsecase struct {
	repo repoconfig.Config
}

// NewConfigUsecase returns a ConfigUsecase backed by the given repository.
func NewConfigUsecase(
	repo repoconfig.Config,
) ConfigUsecase {
	return &configUsecase{repo: repo}
}

func (u *configUsecase) Get(
	_ context.Context,
) (ConfigView, error) {
	configured, err := u.repo.Configured()
	if err != nil {
		return ConfigView{}, fmt.Errorf("get config: read configured: %w", err)
	}

	running := u.repo.Running()

	return ConfigView{
		Running:         running,
		Configured:      configured,
		Defaults:        u.repo.Defaults(),
		RestartRequired: pendingKeys(running, configured),
	}, nil
}

func (u *configUsecase) Patch(
	_ context.Context,
	body json.RawMessage,
) (PatchResult, error) {
	settings, err := flatten(body)
	if err != nil {
		return PatchResult{}, fmt.Errorf("patch config: %w: %w", apperrors.ErrInvalidConfig, err)
	}

	configured, err := u.repo.Configured()
	if err != nil {
		return PatchResult{}, fmt.Errorf("patch config: read configured: %w", err)
	}

	if len(settings) == 0 {
		return PatchResult{}, nil
	}

	next := configured
	applied, rejected := u.set(&next, settings)
	applied, rejected = u.settle(&next, configured, applied, rejected)

	if len(applied) == 0 {
		return PatchResult{Rejected: rejected}, fmt.Errorf(
			"patch config: %w: %s: %s",
			apperrors.ErrInvalidConfig, rejected[0].Key, rejected[0].Message,
		)
	}

	if err := u.repo.Save(next); err != nil {
		return PatchResult{}, fmt.Errorf("patch config: save: %w", err)
	}

	return PatchResult{Applied: applied, Rejected: rejected}, nil
}

// set writes every named setting it can decode, in key order so the report is
// stable, and refuses the rest by name.
func (u *configUsecase) set(
	next *repoconfig.Data,
	settings map[string]json.RawMessage,
) ([]string, []repoconfig.FieldError) {
	def := u.repo.Defaults()

	var (
		applied  []string
		rejected []repoconfig.FieldError
	)

	for _, key := range slices.Sorted(maps.Keys(settings)) {
		if err := repoconfig.SetField(next, def, key, settings[key]); err != nil {
			rejected = append(rejected, repoconfig.FieldError{Key: key, Message: err.Error()})
			continue
		}

		applied = append(applied, key)
	}

	return applied, rejected
}

// settle validates what set produced and withdraws the settings that made it
// invalid, always blaming a key the caller actually sent.
func (u *configUsecase) settle(
	next *repoconfig.Data,
	configured repoconfig.Data,
	applied []string,
	rejected []repoconfig.FieldError,
) ([]string, []repoconfig.FieldError) {
	touched := make(map[string]bool, len(applied))
	for _, key := range applied {
		touched[key] = true
	}

	for _, fe := range u.repo.Validate(*next) {
		blame := blameKey(fe.Key, touched)
		if blame == "" {
			continue
		}

		repoconfig.RestoreField(next, configured, blame)
		rejected = append(rejected, repoconfig.FieldError{Key: blame, Message: fe.Message})
		delete(touched, blame)
	}

	kept := make([]string, 0, len(touched))
	for _, key := range applied {
		if touched[key] {
			kept = append(kept, key)
		}
	}

	return kept, rejected
}

// blameKey attributes a validation failure to a setting the caller actually
// sent. The port range is the one rule spanning two fields, so raising the
// start above an untouched end must be reported against the start.
func blameKey(
	key string,
	touched map[string]bool,
) string {
	if touched[key] {
		return key
	}

	if key == keyPortEnd && touched[keyPortStart] {
		return keyPortStart
	}

	if key == keyPortStart && touched[keyPortEnd] {
		return keyPortEnd
	}

	return ""
}

// pendingKeys lists the settings whose configured value differs from the one
// the process is running with. api.host is excluded: the --host flag can
// override it at start, so the running value is not knowable from
// configuration alone.
func pendingKeys(
	running repoconfig.Data,
	configured repoconfig.Data,
) []string {
	keys := repoconfig.Differing(running, configured)

	return slices.DeleteFunc(keys, func(key string) bool {
		return key == keyHost
	})
}

// flatten turns a sparse configuration document into settings addressed by
// dotted key. Nesting is followed wherever a value is an object, so a document
// shaped like config.yaml needs no per-setting code to read.
func flatten(
	body json.RawMessage,
) (map[string]json.RawMessage, error) {
	settings := make(map[string]json.RawMessage)

	if len(body) == 0 {
		return settings, nil
	}

	if err := descend(body, "", settings); err != nil {
		return nil, err
	}

	return settings, nil
}

func descend(
	raw json.RawMessage,
	prefix string,
	out map[string]json.RawMessage,
) error {
	var section map[string]json.RawMessage
	if err := json.Unmarshal(raw, &section); err != nil {
		return fmt.Errorf("body must be a json configuration object")
	}

	for name, value := range section {
		key := name
		if prefix != "" {
			key = prefix + "." + name
		}

		if isObject(value) {
			if err := descend(value, key, out); err != nil {
				return err
			}
			continue
		}

		out[key] = value
	}

	return nil
}

func isObject(
	raw json.RawMessage,
) bool {
	for _, b := range raw {
		switch b {
		case ' ', '\t', '\n', '\r':
			continue
		case '{':
			return true
		default:
			return false
		}
	}

	return false
}
