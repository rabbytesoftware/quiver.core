package updater

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// StateSchema is the current on-disk updater state schema.
const StateSchema = 1

// Artifact identifies one immutable, content-addressed Quiver executable.
type Artifact struct {
	Version    string `json:"version" yaml:"version"`
	Digest     string `json:"digest" yaml:"digest"`
	Executable string `json:"executable" yaml:"executable"`
}

// NewArtifact returns the canonical artifact identity for version and digest.
func NewArtifact(layout Layout, version, digest string) (Artifact, error) {
	return layout.artifact(version, digest)
}

// Validate confirms that the artifact is canonical and contained by layout.
func (a Artifact) Validate(layout Layout) error {
	expected, err := layout.artifact(a.Version, a.Digest)
	if err != nil {
		return err
	}
	if a.Executable != expected.Executable {
		return fmt.Errorf("updater artifact executable %q: expected %q: %w", a.Executable, expected.Executable, ErrUnsafePath)
	}
	return nil
}

// Selection identifies the active and previous verified Quiver artifacts.
type Selection struct {
	Schema     int       `json:"schema" yaml:"schema"`
	Generation uint64    `json:"generation" yaml:"generation"`
	Current    Artifact  `json:"current" yaml:"current"`
	Previous   *Artifact `json:"previous,omitempty" yaml:"previous,omitempty"`
	PromotedAt time.Time `json:"promoted_at" yaml:"promoted_at"`
}

// Validate confirms that the selection identifies canonical, distinct artifacts.
func (s Selection) Validate(layout Layout) error {
	if s.Schema != StateSchema {
		return fmt.Errorf("updater selection schema %d: expected %d: %w", s.Schema, StateSchema, ErrInvalidState)
	}
	if s.Generation == 0 {
		return fmt.Errorf("updater selection generation: value is required: %w", ErrInvalidState)
	}
	if err := s.Current.Validate(layout); err != nil {
		return fmt.Errorf("updater selection current: %w", err)
	}
	if s.PromotedAt.IsZero() {
		return fmt.Errorf("updater selection promoted_at: value is required: %w", ErrInvalidState)
	}
	if s.Previous == nil {
		return nil
	}
	if err := s.Previous.Validate(layout); err != nil {
		return fmt.Errorf("updater selection previous: %w", err)
	}
	if s.Previous.Digest == s.Current.Digest {
		return fmt.Errorf("updater selection previous: duplicates current digest: %w", ErrInvalidState)
	}
	return nil
}

// Staged identifies a verified candidate awaiting a bootstrap attempt.
type Staged struct {
	Schema     int       `json:"schema" yaml:"schema"`
	Candidate  Artifact  `json:"candidate" yaml:"candidate"`
	PreparedAt time.Time `json:"prepared_at" yaml:"prepared_at"`
}

// Validate confirms that the staged candidate is canonical and timestamped.
func (s Staged) Validate(layout Layout) error {
	if s.Schema != StateSchema {
		return fmt.Errorf("updater staged schema %d: expected %d: %w", s.Schema, StateSchema, ErrInvalidState)
	}
	if err := s.Candidate.Validate(layout); err != nil {
		return fmt.Errorf("updater staged candidate: %w", err)
	}
	if s.PreparedAt.IsZero() {
		return fmt.Errorf("updater staged prepared_at: value is required: %w", ErrInvalidState)
	}
	return nil
}

// EncodeSelection validates and serializes a selection using the stable schema.
func EncodeSelection(layout Layout, selection Selection) ([]byte, error) {
	if err := selection.Validate(layout); err != nil {
		return nil, err
	}
	return encode(selection)
}

// DecodeSelection parses one strict selection document and validates its paths.
func DecodeSelection(layout Layout, data []byte) (Selection, error) {
	var selection Selection
	if err := decode(data, &selection); err != nil {
		return Selection{}, fmt.Errorf("updater selection: decode: %w", err)
	}
	if err := selection.Validate(layout); err != nil {
		return Selection{}, err
	}
	return selection, nil
}

// EncodeStaged validates and serializes a staged candidate using the stable schema.
func EncodeStaged(layout Layout, staged Staged) ([]byte, error) {
	if err := staged.Validate(layout); err != nil {
		return nil, err
	}
	return encode(staged)
}

// DecodeStaged parses one strict staged document and validates its paths.
func DecodeStaged(layout Layout, data []byte) (Staged, error) {
	var staged Staged
	if err := decode(data, &staged); err != nil {
		return Staged{}, fmt.Errorf("updater staged: decode: %w", err)
	}
	if err := staged.Validate(layout); err != nil {
		return Staged{}, err
	}
	return staged, nil
}

func encode(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("updater state: encode: %w", err)
	}
	return append(data, '\n'), nil
}

func decode(data []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}
