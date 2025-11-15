package requirements

import (
    "fmt"
    "github.com/shirou/gopsutil/mem"
)

type ResourceValidator struct {
    MinRAM uint64
}

func (v ResourceValidator) Validate() ValidationResult {
    vm, _ := mem.VirtualMemory()
    if vm.Total < v.MinRAM {
        return ValidationResult{
            Passed: false,
            Messages: []string{
                fmt.Sprintf("Insufficient RAM: %dMB < %dMB", vm.Total/1024/1024, v.MinRAM/1024/1024),
            },
        }
    }
    return ValidationResult{
        Passed:   true,
        Messages: []string{"RAM check passed"},
    }
}
