package forge

import (
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/picker"
)

type target struct {
	Lifecycle lifecycle `yaml:"lifecycle"`
	Expose    expose    `yaml:"expose"`
}

func newTarget(
	name string,
	file string,
	platform domain.OS,
	pick picker.Pick,
) target {
	binary := binaryPath(name, platform.IsWindows())
	return target{
		Lifecycle: lifecycle{Install: installSteps(binary, file, pick)},
		Expose:    newExpose(name, binary, platform, pick),
	}
}
