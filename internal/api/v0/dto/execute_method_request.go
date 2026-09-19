package dto

type ExecuteMethodRequestDTO struct {
	// Variables supplied by the caller. WORKDIR, INSTALL_PATH, ARROW_NAMESPACE,
	// PLATFORM and REF are computed by Quiver and rejected with 400 if set.
	Variables map[string]string `json:"variables" yaml:"variables"`
}
