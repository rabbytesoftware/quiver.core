package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFTSQuery(t *testing.T) {
	testCases := []struct {
		name string
		text string
		want string
	}{
		{"one term", "bruno", `"bruno"`},
		{"separators split terms", "quiver-chat", `"quiver" AND "chat"`},
		{"short terms are dropped", "ui-kit", `"kit"`},
		{"only short terms falls back to the literal phrase", "ui", `"ui"`},
		{"only separators falls back to the literal phrase", "---", `"---"`},
		{"quotes are doubled inside a term", `say"hi`, `"say""hi"`},
		{"unicode counts runes, not bytes", "ñandú", `"ñandú"`},
		{"two unicode letters are still too short", "ñá", `"ñá"`},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ftsQuery(tc.text))
		})
	}
}
