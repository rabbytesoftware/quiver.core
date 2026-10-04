//go:build integration

package kit

import "os"

// quiverHomeEnv is the variable internal/core/metadata resolves the whole home
// tree from, config.yaml included, ahead of the process-level HOME.
const quiverHomeEnv = "QUIVER_HOME"

// isolateHome points the process-wide config and metadata at a fresh, empty
// home and returns the function that removes it.
//
// config.Get reads <home>/config.yaml exactly once per process and caches it,
// and a suite that never overrides the home therefore reads the developer's
// real ~/.quiver/config.yaml. A setting there (self_update_channel, intervals,
// hosts) silently changes what the daemon under test does: with
// self_update_channel set, the self-arrow registers under that channel instead
// of the one the test stamped on the build. CI has no such file, so the suite
// would pass there and fail on a developer machine.
func isolateHome() (func(), error) {
	dir, err := os.MkdirTemp("", "qv-kit-home-*")
	if err != nil {
		return nil, err
	}
	if err := os.Setenv(quiverHomeEnv, dir); err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	return func() { _ = os.RemoveAll(dir) }, nil
}
