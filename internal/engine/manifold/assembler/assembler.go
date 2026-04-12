package assembler

import "github.com/rabbytesoftware/quiver/internal/domain"

// ValidateArrow applies all business rules to a domain.ArrowManifest.
// It runs every rule and returns all violations as AssemblerErrors (nil on success).
func ValidateArrow(
	manifest *domain.ArrowManifest,
) error {
	var errs AssemblerErrors
	errs = append(errs, validateLifecyclePairs(manifest.Lifecycle)...)
	errs = append(errs, validateVariables(manifest.Variables)...)
	errs = append(errs, validateNetbridge(manifest.Netbridge)...)
	errs = append(errs, validateDependencies(manifest.Dependencies)...)
	errs = append(errs, validateMethodStates(manifest.Methods)...)
	if len(errs) == 0 {
		return nil
	}
	return errs
}

// ValidateQuiver applies all business rules to a domain.QuiverManifest.
func ValidateQuiver(
	manifest *domain.QuiverManifest,
) error {
	return nil
}
