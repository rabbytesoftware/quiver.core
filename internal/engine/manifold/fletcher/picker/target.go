package picker

import (
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type target struct {
	family string
	arch   string
}

func targetOf(
	platform domain.OS,
) target {
	switch platform {
	case domain.OSLinuxAMD64:
		return target{family: familyLinux, arch: archAMD64}
	case domain.OSLinuxARM64:
		return target{family: familyLinux, arch: archARM64}
	case domain.OSDarwinAMD64:
		return target{family: familyDarwin, arch: archAMD64}
	case domain.OSDarwinARM64:
		return target{family: familyDarwin, arch: archARM64}
	case domain.OSWindowsAMD64:
		return target{family: familyWindows, arch: archAMD64}
	case domain.OSWindowsARM64:
		return target{family: familyWindows, arch: archARM64}
	}
	return target{}
}

func (t target) contains(
	c classification,
) bool {
	return c.family == t.family
}

func (t target) native(
	c classification,
) bool {
	return c.arch == t.arch
}

func (t target) universal(
	c classification,
) bool {
	return c.arch == archUniversal && t.family == familyDarwin
}

func (t target) assumesArch() bool {
	return t.family != familyLinux || t.arch != archARM64
}

func (t target) emulates() bool {
	return t.arch == archARM64 && t.family != familyLinux
}
