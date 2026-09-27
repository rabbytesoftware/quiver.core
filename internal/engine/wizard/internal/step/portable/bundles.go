package portable

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

const bundleSuffix = ".app"

func bundleApps(
	to string,
	names []string,
) []domain.PortableApp {
	apps := make([]domain.PortableApp, 0, len(names))
	for _, name := range names {
		entry := filepath.Join(to, name)
		if !strings.HasSuffix(name, bundleSuffix) || !isDir(entry) {
			continue
		}
		apps = append(apps, domain.PortableApp{
			Name:  strings.TrimSuffix(name, bundleSuffix),
			Entry: entry,
		})
	}

	return apps
}

func isDir(
	path string,
) bool {
	info, err := os.Lstat(path)

	return err == nil && info.IsDir()
}
