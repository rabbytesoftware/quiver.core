package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type markerKey struct{}

func TestOutsideHandler_DropsValuesKeepsCancellation(t *testing.T) {
	testCases := []struct {
		name         string
		withDeadline bool
	}{
		{name: "no deadline"},
		{name: "with deadline", withDeadline: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			parent, cancelParent := context.WithCancel(context.WithValue(context.Background(), markerKey{}, "x"))
			defer cancelParent()
			if tc.withDeadline {
				var cancelDeadline context.CancelFunc
				parent, cancelDeadline = context.WithTimeout(parent, time.Hour)
				defer cancelDeadline()
			}

			ctx, release := outsideHandler(parent)
			defer release()

			assert.Nil(t, ctx.Value(markerKey{}))
			_, hasDeadline := ctx.Deadline()
			assert.Equal(t, tc.withDeadline, hasDeadline)
			require.NoError(t, ctx.Err())

			cancelParent()
			select {
			case <-ctx.Done():
			case <-time.After(time.Second):
				t.Fatal("derived context did not follow the parent's cancellation")
			}
		})
	}
}

func TestOutsideHandler_ReleaseCancels(t *testing.T) {
	ctx, release := outsideHandler(context.Background())
	release()
	require.Error(t, ctx.Err())
}
