package domain

import (
	"fmt"

	"github.com/google/uuid"
)

const (
	MaxNameLength        = 255
	MaxDescriptionLength = 1000
)

type Arrow struct {
	ID            uuid.UUID `json:"id"`
	Namespace     Namespace `json:"namespace"`
	ArrowVersion  []string  `json:"arrow_version" gorm:"serializer:json"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	Version       string    `json:"version"`
	License       string    `json:"license"`
	Maintainers   []string  `json:"maintainers" gorm:"serializer:json"`
	Credits       []string  `json:"credits" gorm:"serializer:json"`
	URL           URL       `json:"url"`
	Documentation string    `json:"documentation"`
	QuiverURL     URL       `json:"quiver_url"`
	IconURL       URL       `json:"icon_url"`
	BannerURL     URL       `json:"banner_url"`

	Requirements Requirement `json:"requirements" gorm:"serializer:json"`
	Dependencies []Namespace `json:"dependencies" gorm:"serializer:json"`

	Netbridge []PortRule `json:"netbridge" gorm:"serializer:json"`
	Variables []Variable `json:"variables" gorm:"serializer:json"`

	Methods []Method `json:"methods" gorm:"serializer:json"`
}

func (a *Arrow) Validate() error {
	if a.Name == "" {
		return fmt.Errorf("arrow name cannot be empty")
	}
	if len(a.Name) > MaxNameLength {
		return fmt.Errorf("arrow name exceeds max length of %d", MaxNameLength)
	}

	if a.Version == "" {
		return fmt.Errorf("arrow version cannot be empty")
	}

	if len(a.Description) > MaxDescriptionLength {
		return fmt.Errorf("description exceeds max length of %d", MaxDescriptionLength)
	}

	if a.Namespace != "" {
		if err := a.Namespace.Validate(); err != nil {
			return fmt.Errorf("namespace validation failed: %w", err)
		}
	}

	if err := a.Requirements.Validate(); err != nil {
		return fmt.Errorf("requirements validation failed: %w", err)
	}

	if a.IconURL != "" && !a.IconURL.IsValid() {
		return fmt.Errorf("invalid icon URL: %s", a.IconURL)
	}

	if a.BannerURL != "" && !a.BannerURL.IsValid() {
		return fmt.Errorf("invalid banner URL: %s", a.BannerURL)
	}

	for i, v := range a.Variables {
		if err := v.Validate(); err != nil {
			return fmt.Errorf("variable[%d] validation failed: %w", i, err)
		}
	}

	for i, m := range a.Methods {
		if err := m.Validate(); err != nil {
			return fmt.Errorf("method[%d] validation failed: %w", i, err)
		}
	}

	for i, p := range a.Netbridge {
		if err := p.Validate(); err != nil {
			return fmt.Errorf("port[%d] validation failed: %w", i, err)
		}
	}

	return nil
}
