package recommendation_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/recommendation"
)

func TestParseWindow(t *testing.T) {
	testCases := []struct {
		name    string
		raw     string
		want    time.Duration
		wantErr bool
	}{
		{name: "empty means no window", raw: "", want: 0},
		{name: "days", raw: "90d", want: 90 * 24 * time.Hour},
		{name: "zero days", raw: "0d", want: 0},
		{name: "go duration", raw: "36h", want: 36 * time.Hour},
		{name: "negative days", raw: "-1d", wantErr: true},
		{name: "fractional days", raw: "1.5d", wantErr: true},
		{name: "not a number of days", raw: "xd", wantErr: true},
		{name: "negative duration", raw: "-5h", wantErr: true},
		{name: "garbage", raw: "soon", wantErr: true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := recommendation.ParseWindow(tc.raw)

			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
