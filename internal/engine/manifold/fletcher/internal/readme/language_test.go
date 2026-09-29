package readme

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsCJKDominant_Classification(
	t *testing.T,
) {
	testCases := []struct {
		name string
		raw  string
		want bool
	}{
		{name: "english prose", raw: "ripgrep is a line-oriented search tool.", want: false},
		{name: "chinese dominant", raw: "这是一个非常快速的搜索工具,支持正则表达式", want: true},
		{name: "mixed mostly english with a code name", raw: "bat supports 中文 too but is mostly English prose here", want: false},
		{name: "empty", raw: "", want: false},
		{name: "japanese dominant", raw: "これは日本語のテキストです。とても長い説明文", want: true},
		{name: "korean dominant", raw: "이것은 한국어 텍스트입니다 매우 긴 설명문입니다", want: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, IsCJKDominant([]byte(tc.raw)))
		})
	}
}

func TestSelectReadme_PrefersEnglishWhenDefaultIsCJK(
	t *testing.T,
) {
	byName := map[string][]byte{
		"README.md":    []byte("这是一个非常快速的搜索工具,支持正则表达式"),
		"README.en.md": []byte("This is a very fast search tool with regex support."),
	}

	got := SelectReadme(byName)

	assert.Equal(t, byName["README.en.md"], got)
}

func TestSelectReadme_KeepsDefaultWhenNotCJK(
	t *testing.T,
) {
	byName := map[string][]byte{
		"README.md":    []byte("plain english prose"),
		"README.en.md": []byte("should not be picked"),
	}

	got := SelectReadme(byName)

	assert.Equal(t, byName["README.md"], got)
}

func TestSelectReadme_FallsBackToDefaultWhenNoEnglishVariant(
	t *testing.T,
) {
	byName := map[string][]byte{
		"README.md": []byte("这是一个非常快速的搜索工具,支持正则表达式"),
	}

	got := SelectReadme(byName)

	assert.Equal(t, byName["README.md"], got)
}

func TestSelectReadme_FallsBackToKnownNameWhenNoDefault(
	t *testing.T,
) {
	byName := map[string][]byte{
		"Readme.md": []byte("some content"),
	}

	got := SelectReadme(byName)

	assert.Equal(t, byName["Readme.md"], got)
}

func TestSelectReadme_NilWhenNothingKnown(
	t *testing.T,
) {
	got := SelectReadme(map[string][]byte{"OTHER.md": []byte("x")})

	assert.Nil(t, got)
}

func TestSelectReadme_PrefersLongFormEnglishVariant(
	t *testing.T,
) {
	byName := map[string][]byte{
		"README.md":         []byte("这是一个非常快速的搜索工具,支持正则表达式"),
		"README.english.md": []byte("This is a very fast search tool with regex support."),
	}

	got := SelectReadme(byName)

	assert.Equal(t, byName["README.english.md"], got)
}
