package providers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBitbucket_Search_Unmarked_ReturnsUnsupported(t *testing.T) {
	_, err := NewBitbucket(Config{Host: "bitbucket.org"}).
		Search(context.Background(), SearchRequest{Text: "x", Unmarked: true, MinStars: 50})

	require.ErrorIs(t, err, ErrSearchUnsupported)
}
