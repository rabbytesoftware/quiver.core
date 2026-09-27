package forge

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExposeName(t *testing.T) {
	testCases := []struct {
		name string
		repo string
		want string
	}{
		{name: "valid name is kept", repo: "ripgrep", want: "ripgrep"},
		{name: "allowed punctuation is kept", repo: "Foo_Bar.baz+1-x", want: "Foo_Bar.baz+1-x"},
		{name: "invalid characters become dashes", repo: "my tool/ü", want: "my-tool--"},
		{name: "leading punctuation is trimmed", repo: "._-dotfiles", want: "dotfiles"},
		{name: "long names are cut to 64", repo: strings.Repeat("a", 70), want: strings.Repeat("a", 64)},
		{name: "nothing usable falls back", repo: "...", want: "app"},
		{name: "empty falls back", repo: "", want: "app"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := exposeName(tc.repo)

			assert.Equal(t, tc.want, got)
			assert.Regexp(t, `^[A-Za-z0-9][A-Za-z0-9._+-]{0,63}$`, got)
		})
	}
}
