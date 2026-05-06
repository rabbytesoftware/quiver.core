//go:build integration

package kit

import (
	"fmt"
)

// NSFor constructs a versioned test namespace.
// fixture: relative path under testdata/arrows/ e.g. "quiver-test/tool-a"
// tag: git tag e.g. "v1"
// Returns: "quiver.test/quiver-test/tool-a@v1"
func NSFor(fixture, tag string) string {
	return "quiver.test/" + fixture + "@" + tag
}

// NSForGlob constructs a glob-constrained namespace.
// glob: constraint pattern e.g. "v*"
func NSForGlob(fixture, glob string) string {
	return "quiver.test/" + fixture + "@" + glob
}

// QuiverNSFor constructs a versioned quiver test namespace.
// fixture: key under testdata/quivers/ e.g. "gaming-quiver"
// tag: git tag e.g. "v1"
// Returns: "quiver.test/quiver-test/gaming-quiver@v1"
func QuiverNSFor(fixture, tag string) string {
	return "quiver.test/quiver-test/" + fixture + "@" + tag
}

// BuildMinimalYAML produces a minimal valid arrow manifest with the given name.
func BuildMinimalYAML(name string) []byte {
	return []byte(fmt.Sprintf(`schema: "arrow@v0"
metadata:
  name: %s
  version: 1.0.0
  description: test
targets:
  "*":
    lifecycle:
      install:
        - type: run
          command: echo installed
          title: Install
          timeout: 10s
          exit_on_failure: true
      uninstall:
        - type: run
          command: echo uninstalled
          title: Uninstall
          timeout: 10s
          exit_on_failure: false
`, name))
}
