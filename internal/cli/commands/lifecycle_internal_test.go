package commands

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestIsLifecycle(t *testing.T) {
	a := &app{deps: Deps{IsTTY: func() bool { return false }}}

	if !IsLifecycle(a.installCmd()) {
		t.Error("install must be annotated lifecycle")
	}
	if !IsLifecycle(a.uninstallCmd()) {
		t.Error("uninstall must be annotated lifecycle")
	}
	if IsLifecycle(a.arrowCmd()) {
		t.Error("arrow (catalog) must NOT be lifecycle")
	}
	if IsLifecycle(nil) {
		t.Error("nil command is not lifecycle")
	}
	if IsLifecycle(&cobra.Command{}) {
		t.Error("un-annotated command is not lifecycle")
	}
}
