package assembler

import "github.com/rabbytesoftware/quiver/internal/domain"

func ValidateArrow(
	manifest *domain.ArrowManifest,
) error {
	if err := validateLifecyclePairs(manifest.Lifecycle); err != nil {
		return err
	}

	if err := validateVariables(manifest.Variables); err != nil {
		return err
	}

	if err := validateNetbridge(manifest.Netbridge); err != nil {
		return err
	}

	if err := validateDependencies(manifest.Dependencies); err != nil {
		return err
	}

	return validateMethodStates(manifest.Methods)
}

// ValidateQuiver applies all business rules to a domain.QuiverManifest
// and returns an error if any rules are violated.
func ValidateQuiver(
	manifest *domain.QuiverManifest,
) error {
	// Quiver currently has no complex validation rules.
	return nil
}
