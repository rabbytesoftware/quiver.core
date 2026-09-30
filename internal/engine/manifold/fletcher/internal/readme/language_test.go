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
