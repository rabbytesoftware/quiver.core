package domain

import (
	goruntime "runtime"
)

type OS string

const (
	OSLinuxAMD64 OS = "linux/amd64"
	OSLinuxARM64 OS = "linux/arm64"

	OSWindowsAMD64 OS = "windows/amd64"
	OSWindowsARM64 OS = "windows/arm64"

	OSDarwinAMD64 OS = "darwin/amd64"
	OSDarwinARM64 OS = "darwin/arm64"
)

func (o OS) String() string {
	return string(o)
}

func (o OS) IsValid() bool {
	return o == OSLinuxAMD64 ||
		o == OSLinuxARM64 ||
		o == OSWindowsAMD64 ||
		o == OSWindowsARM64 ||
		o == OSDarwinAMD64 ||
		o == OSDarwinARM64
}

func (o OS) IsLinux() bool {
	return o == OSLinuxAMD64 || o == OSLinuxARM64
}

func (o OS) IsWindows() bool {
	return o == OSWindowsAMD64 || o == OSWindowsARM64
}

func (o OS) IsDarwin() bool {
	return o == OSDarwinAMD64 || o == OSDarwinARM64
}

func (o OS) IsAMD64() bool {
	return o == OSLinuxAMD64 || o == OSWindowsAMD64 || o == OSDarwinAMD64
}

func (o OS) IsARM64() bool {
	return o == OSLinuxARM64 || o == OSWindowsARM64 || o == OSDarwinARM64
}

// CurrentOS maps runtime.GOOS + runtime.GOARCH to the domain OS enum.
// Unrecognised combinations are returned as-is (e.g. "freebsd/amd64").
func CurrentOS() OS {
	return OS(goruntime.GOOS + "/" + goruntime.GOARCH)
}

func AllOS() []OS {
	return []OS{
		OSLinuxAMD64,
		OSLinuxARM64,
		OSWindowsAMD64,
		OSWindowsARM64,
		OSDarwinAMD64,
		OSDarwinARM64,
	}
}
