package gather

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestSources_FirstError_FollowsFetchOrder(t *testing.T) {
	errPicks := errors.New("picks")
	errPage := errors.New("page")
	errReadme := errors.New("readme")
	testCases := []struct {
		name    string
		src     sources
		want    error
		notWant []error
	}{
		{
			name:    "picks win",
			src:     sources{picksErr: errPicks, pageErr: errPage, readmeErr: errReadme},
			want:    errPicks,
			notWant: []error{errPage, errReadme},
		},
		{
			name:    "page wins over readme",
			src:     sources{pageErr: errPage, readmeErr: errReadme},
			want:    errPage,
			notWant: []error{errReadme},
		},
		{
			name: "readme",
			src:  sources{readmeErr: errReadme},
			want: errReadme,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.src.firstError(domain.Namespace("github.com/acme/tool"))

			assert.ErrorIs(t, err, tc.want)
			for _, other := range tc.notWant {
				assert.NotErrorIs(t, err, other)
			}
		})
	}
	assert.NoError(t, sources{}.firstError(domain.Namespace("github.com/acme/tool")))
}
