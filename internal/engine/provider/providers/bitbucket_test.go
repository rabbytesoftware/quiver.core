package providers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBitbucket_Host_ReturnsConfiguredHost(t *testing.T) {
	assert.Equal(t, "bitbucket.org", NewBitbucket(Config{Host: "bitbucket.org"}).Host())
}

func TestBitbucket_CanSearch_AlwaysFalse(t *testing.T) {
	assert.False(t, NewBitbucket(Config{Host: "bitbucket.org"}).CanSearch())
}

func TestBitbucket_Search_ReturnsUnsupported(t *testing.T) {
	_, err := NewBitbucket(Config{Host: "bitbucket.org"}).
		Search(context.Background(), SearchRequest{Text: "x"})

	require.ErrorIs(t, err, ErrSearchUnsupported)
}

func TestBitbucket_Search_Unmarked_ReturnsUnsupported(t *testing.T) {
	_, err := NewBitbucket(Config{Host: "bitbucket.org"}).
		Search(context.Background(), SearchRequest{Text: "x", Unmarked: true, MinStars: 50})

	require.ErrorIs(t, err, ErrSearchUnsupported)
}
