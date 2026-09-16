package commands

import (
	"testing"

	"github.com/spf13/cobra"
)

// TestIsLifecycle exercises IsLifecycle/AnnotationLifecycle directly on a
// hand-built command rather than through a lifecycle command's constructor:
// every command that used to carry the annotation (install, uninstall, ...)
// has moved to commands/runtime, leaving no vehicle for it in this package.
func TestIsLifecycle(t *testing.T) {
	lifecycleCmd := &cobra.Command{Annotations: map[string]string{AnnotationLifecycle: "true"}}
	if !IsLifecycle(lifecycleCmd) {
		t.Error("a command annotated quiver_lifecycle=true must be lifecycle")
	}
	if IsLifecycle(&cobra.Command{}) {
		t.Error("un-annotated command is not lifecycle")
	}
	if IsLifecycle(nil) {
		t.Error("nil command is not lifecycle")
	}
}
