package requirements

import (
    "fmt"
    "os/exec"
)

type DependencyValidator struct {
    Dependencies []string
}

func (v DependencyValidator) Validate() ValidationResult {
    var messages []string
    passed := true

    for _, dep := range v.Dependencies {
        if _, err := exec.LookPath(dep); err != nil {
            passed = false
            messages = append(messages, fmt.Sprintf("Missing dependency: %s", dep))
        }
    }

    return ValidationResult{
        Passed:   passed,
        Messages: messages,
    }
}
