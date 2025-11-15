package requirements

type SystemRequirementsValidator struct {
    OSValidator         OSValidator
    ResourceValidator   ResourceValidator
    DependencyValidator DependencyValidator
}

func (srv SystemRequirementsValidator) CheckOS() ValidationResult {
    return srv.OSValidator.Validate()
}

func (srv SystemRequirementsValidator) CheckResources() ValidationResult {
    return srv.ResourceValidator.Validate()
}

func (srv SystemRequirementsValidator) CheckDependencies() ValidationResult {
    return srv.DependencyValidator.Validate()
}

func (srv SystemRequirementsValidator) RunAllChecks() ValidationResult {
    results := []ValidationResult{
        srv.CheckOS(),
        srv.CheckResources(),
        srv.CheckDependencies(),
    }

    final := ValidationResult{Passed: true}
    for _, r := range results {
        if !r.Passed {
            final.Passed = false
        }
        final.Messages = append(final.Messages, r.Messages...)
    }

    return final
}
