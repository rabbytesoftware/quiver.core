package ws_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/api/ws"
)

func TestNamespaceMatch(t *testing.T) {
	testCases := []struct {
		name    string
		pattern string
		value   string
		want    bool
	}{
		{
			// The whole point: a client subscribes with the namespace a user
			// typed, and the events carry the ref the arrow is catalogued under.
			name:    "bare pattern covers a versioned namespace",
			pattern: "github.com/user/app",
			value:   "github.com/user/app@main",
			want:    true,
		},
		{
			name:    "bare pattern matches the bare namespace",
			pattern: "github.com/user/app",
			value:   "github.com/user/app",
			want:    true,
		},
		{
			name:    "an explicit ref still selects only that ref",
			pattern: "github.com/user/app@v1",
			value:   "github.com/user/app@v2",
			want:    false,
		},
		{
			name:    "an explicit ref matches itself",
			pattern: "github.com/user/app@v1",
			value:   "github.com/user/app@v1",
			want:    true,
		},
		{
			// A bare pattern must not swallow a different arrow whose name
			// merely starts with the same text.
			name:    "bare pattern does not match a sibling arrow",
			pattern: "github.com/user/app",
			value:   "github.com/user/app-extra@main",
			want:    false,
		},
		{
			name:    "glob still selects across an org",
			pattern: "github.com/user/*",
			value:   "github.com/user/app",
			want:    true,
		},
		{
			name:    "empty pattern matches everything",
			pattern: "",
			value:   "github.com/user/app@main",
			want:    true,
		},
		{
			name:    "unrelated namespace does not match",
			pattern: "github.com/user/app",
			value:   "github.com/other/thing@main",
			want:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ws.NamespaceMatch(tc.pattern, tc.value))
		})
	}
}
