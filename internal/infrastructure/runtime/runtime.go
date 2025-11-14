package runtime

import (
	"bytes"
	"fmt"
	"os/exec"
	"runtime"

	"github.com/google/uuid"
	"github.com/rabbytesoftware/quiver/internal/models/system"
)

type ProcessInfo struct {
	Id        uuid.UUID
	Cmd       *exec.Cmd
	Name      string
	Status    string // "running", "stopped", "finished", "stopping", "killing"
	Output    *bytes.Buffer
	Error     *bytes.Buffer
	OutChan   chan string
	ErrorChan chan string
	DoneChan  chan struct{}
	ExitErr   error
	ExitCode  int
}

type Runtime struct {
	REEImplementation REEInterface
	CurrentOS         system.OS
}

func NewRuntime() REEInterface {
	r := &Runtime{}

	// set OS global variable
	os := system.OS(runtime.GOOS + "/" + runtime.GOARCH)

	// check if OS is supported
	if os.IsValid() {
		r.CurrentOS = os
	} else {
		fmt.Println("Unsupported operative system:", os)
		r.CurrentOS = ""

		return nil
	}

	processes := make(map[string]*ProcessInfo)

	// handle each OS
	if r.CurrentOS.IsWindows() {
		return &WindowsRuntime{Runtime: r, processes: processes}
	} else if r.CurrentOS.IsLinux() {
		return &LinuxRuntime{Runtime: r, processes: processes}
	} else if r.CurrentOS.IsDarwin() {
		return &DarwinRuntime{Runtime: r, processes: processes}
	}

	return nil
}
