package store

import (
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// ChannelOf exposes the unexported channelOf function for white-box tests.
func ChannelOf(
	arrow domain.Arrow,
) string {
	return channelOf(arrow)
}
