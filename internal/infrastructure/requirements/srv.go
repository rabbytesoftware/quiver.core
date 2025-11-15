package requirements

type ValidationResult struct {
    Passed   bool
    Messages []string
}

type SRV interface {
    CheckOS() ValidationResult
    CheckResources() ValidationResult
    CheckDependencies() ValidationResult
    RunAllChecks() ValidationResult
}
