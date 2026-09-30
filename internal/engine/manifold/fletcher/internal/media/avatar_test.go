package media

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/hosts"
)

type avatarHost struct {
	hosts.Host
	url string
}

func (h avatarHost) OwnerAvatarURL(
	_ domain.Namespace,
) string {
	return h.url
}

func TestProbeAvatar_ImageReturnsTheStableURL(t *testing.T) {
	fetch := func(_ context.Context, _ string) ([]byte, error) {
		return encodePNG(t, 460, 460), nil
	}

	got := ProbeAvatar(context.Background(), fetch, avatarHost{url: "https://h.test/o.png"}, "github.com/o/r")

	assert.Equal(t, "https://h.test/o.png", got)
}

func TestProbeAvatar_MissesAreEmpty(t *testing.T) {
	testCases := []struct {
		name  string
		url   string
		fetch Fetch
	}{
		{
			name:  "host without avatars",
			url:   "",
			fetch: func(_ context.Context, _ string) ([]byte, error) { return encodePNG(t, 460, 460), nil },
		},
		{
			name:  "fetch failure",
			url:   "https://h.test/o.png",
			fetch: func(_ context.Context, _ string) ([]byte, error) { return nil, errors.New("down") },
		},
		{
			name:  "not an image",
			url:   "https://h.test/o.png",
			fetch: func(_ context.Context, _ string) ([]byte, error) { return []byte("<html></html>"), nil },
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := ProbeAvatar(context.Background(), tc.fetch, avatarHost{url: tc.url}, "github.com/o/r")

			assert.Empty(t, got)
		})
	}
}
