package shelf

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApplyRequest_Relocate(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "wd", "Tool.app")
	dest := filepath.Join(root, "Applications", "Tool.app")
	req := applyRequest{moved: map[string]string{src: dest}}

	testCases := []struct {
		name   string
		req    applyRequest
		target string
		want   string
	}{
		{name: "no moves", req: applyRequest{}, target: filepath.Join(src, "x"), want: filepath.Join(src, "x")},
		{name: "inside moved bundle", req: req, target: filepath.Join(src, "Contents", "MacOS", "tool"), want: filepath.Join(dest, "Contents", "MacOS", "tool")},
		{name: "moved bundle itself", req: req, target: src, want: dest},
		{name: "outside", req: req, target: filepath.Join(root, "wd", "tool"), want: filepath.Join(root, "wd", "tool")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.req.relocate(tc.target))
		})
	}
}
