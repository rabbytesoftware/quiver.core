package ownership

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/mocks"
)

func TestHolder_Refusal(t *testing.T) {
	testCases := []struct {
		name string
		h    Holder
		want string
	}{
		{name: "absent", h: Holder{}, want: ""},
		{name: "same owner", h: Holder{Exists: true, Namespace: mocks.BareA}, want: ""},
		{name: "unmanaged", h: Holder{Exists: true}, want: models.ReasonUnmanaged},
		{name: "foreign", h: Holder{Exists: true, Namespace: mocks.BareB}, want: "owned by github.com/other/thing"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.h.Refusal(mocks.BareA))
		})
	}
}

func TestUnowned(t *testing.T) {
	assert.Equal(t, Holder{}, Unowned("/anything"))
}
