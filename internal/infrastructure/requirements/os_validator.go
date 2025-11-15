package requirements

import (
    "fmt"
    "runtime"
)

type OSValidator struct {
    SupportedOS   []string
    SupportedArch []string
}

func (v OSValidator) Validate() ValidationResult {
    var messages []string
    osValid := false
    archValid := false

    for _, os := range v.SupportedOS {
        if runtime.GOOS == os {
            osValid = true
            break
        }
    }
    if !osValid {
        messages = append(messages, fmt.Sprintf("Unsupported Operating System: %s", runtime.GOOS))
    }

    for _, arch := range v.SupportedArch {
        if runtime.GOARCH == arch {
            archValid = true
            break
        }
    }
    if !archValid {
        messages = append(messages, fmt.Sprintf("Unsupported Architecture: %s", runtime.GOARCH))
    }

    return ValidationResult{
        Passed:   osValid && archValid,
        Messages: messages,
    }
}
