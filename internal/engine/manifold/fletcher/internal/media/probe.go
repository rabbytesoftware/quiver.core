package media

import (
	"context"
	"strings"
	"sync"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/hosts"
)

func ProbeIcon(
	ctx context.Context,
	fetch Fetch,
	host hosts.Host,
	ns domain.Namespace,
	ref string,
) string {
	urls := fileURLsOf(host, ns, ref)
	paths := iconProbePaths()
	accepted := make([]string, len(paths))

	var wg sync.WaitGroup
	for i, path := range paths {
		url, ok := urls.pinned(path)
		if !ok {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			data, ok := fetchProbe(ctx, fetch, url)
			if ok && acceptProbedIcon(path, data) {
				accepted[i] = url
			}
		}()
	}
	wg.Wait()

	for _, icon := range accepted {
		if icon != "" {
			return icon
		}
	}
	return ""
}

func iconProbePaths() []string {
	return []string{
		"src-tauri/icons/icon.png",
		"build/icon.png",
		"logo.svg",
	}
}

func isSVGPath(
	path string,
) bool {
	return strings.HasSuffix(strings.ToLower(path), ".svg")
}

func acceptProbedIcon(
	path string,
	data []byte,
) bool {
	if isSVGPath(path) {
		return true
	}
	dim, ok := sniff(data)
	if !ok {
		return false
	}
	return dim.Width == dim.Height && dim.Width >= 128
}
