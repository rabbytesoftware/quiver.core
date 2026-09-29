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
	paths := IconProbePaths()
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

func IconProbePaths() []string {
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

func isRejectedIconPath(
	path string,
) bool {
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, ".ico") || strings.HasSuffix(lower, ".icns") {
		return true
	}
	return strings.Contains(lower, "favicon")
}

func acceptProbedIcon(
	path string,
	data []byte,
) bool {
	if isRejectedIconPath(path) {
		return false
	}
	if isSVGPath(path) {
		return true
	}
	dim, ok := Sniff(data)
	if !ok {
		return false
	}
	return dim.Width == dim.Height && dim.Width >= 128
}
