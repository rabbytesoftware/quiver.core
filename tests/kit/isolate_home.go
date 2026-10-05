//go:build integration

package kit

import "os"

const quiverHomeEnv = "QUIVER_HOME"

func isolateHome() (func(), error) {
	dir, err := os.MkdirTemp("", "qv-kit-home-*")
	if err != nil {
		return nil, err
	}
	if err := os.Setenv(quiverHomeEnv, dir); err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	return func() {
		_ = os.RemoveAll(dir)
	}, nil
}
