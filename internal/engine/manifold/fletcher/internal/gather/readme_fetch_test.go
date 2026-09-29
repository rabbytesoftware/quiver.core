package gather_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/picker"
)

func TestDrafter_Draft_ReadmeSelection(t *testing.T) {
	cjk := "这是一个非常快速的搜索工具,支持正则表达式"
	testCases := []struct {
		name     string
		files    map[string]string
		want     string
		wantRead []string
	}{
		{
			name:     "README.md wins",
			files:    map[string]string{"README.md": "Upper.", "readme.md": "Lower."},
			want:     "Upper.",
			wantRead: []string{"README.md"},
		},
		{
			name:     "lowercase readme is second",
			files:    map[string]string{"readme.md": "Lower.", "README": "Bare."},
			want:     "Lower.",
			wantRead: []string{"README.md", "readme.md"},
		},
		{
			name:     "markdown extension is third",
			files:    map[string]string{"README.markdown": "Long ext.", "README": "Bare."},
			want:     "Long ext.",
			wantRead: []string{"README.md", "readme.md", "README.markdown"},
		},
		{
			name:     "bare README is last",
			files:    map[string]string{"README": "Bare."},
			want:     "Bare.",
			wantRead: []string{"README.md", "readme.md", "README.markdown", "README"},
		},
		{
			name:     "missing readme falls back to description",
			files:    map[string]string{},
			want:     "Tool description.",
			wantRead: []string{"README.md", "readme.md", "README.markdown", "README"},
		},
		{
			name:     "cjk default prefers english variant",
			files:    map[string]string{"README.md": cjk, "README_EN.md": "English."},
			want:     "English.",
			wantRead: []string{"README.md", "README.en.md", "README_EN.md"},
		},
		{
			name:     "cjk lowercase default prefers long english variant",
			files:    map[string]string{"readme.md": cjk, "README.english.md": "Long English."},
			want:     "Long English.",
			wantRead: []string{"README.md", "readme.md", "README.en.md", "README_EN.md", "README.english.md"},
		},
		{
			name:     "cjk default without english keeps default",
			files:    map[string]string{"README.md": cjk},
			want:     cjk,
			wantRead: []string{"README.md", "README.en.md", "README_EN.md", "README.english.md"},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			files := make(map[string][]byte, len(tc.files))
			for name, content := range tc.files {
				files[name] = []byte(content)
			}
			host := &stubHost{
				assets: realAssets(),
				page:   pageWith("og:description", "Tool description."),
				files:  files,
			}

			manifest, err := newDrafter(t, host, picker.New()).Draft(context.Background(), testNS, testTag)
			require.NoError(t, err)

			arrow := parse(t, manifest)
			assert.Equal(t, tc.want, arrow.Readme)
			assert.Equal(t, tc.wantRead, host.readmeReads()[:len(tc.wantRead)])
		})
	}
}
