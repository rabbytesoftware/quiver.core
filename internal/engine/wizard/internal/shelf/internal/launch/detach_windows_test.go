//go:build windows

package launch

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreationFlags(t *testing.T) {
	testCases := []struct {
		name      string
		target    string
		breakaway bool
		want      uint32
	}{
		{"gui program", `C:\apps\tool.exe`, false, createNewProcessGroup | detachedProcess},
		{"gui program with breakaway", `C:\apps\tool.exe`, true, createNewProcessGroup | detachedProcess | createBreakaway},
		{"batch script gets no console instead of being detached", `C:\apps\tool.BAT`, false, createNewProcessGroup | createNoWindow},
		{"cmd script", `C:\apps\tool.cmd`, true, createNewProcessGroup | createNoWindow | createBreakaway},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, creationFlags(tc.target, tc.breakaway))
		})
	}
}
