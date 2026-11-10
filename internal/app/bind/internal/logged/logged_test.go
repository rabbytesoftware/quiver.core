package logged

import (
	"context"
	"errors"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
)

type agg struct{ N int }

func TestHandler(t *testing.T) {
	testCases := []struct {
		name string
		err  error
	}{
		{name: "success is silent"},
		{name: "failure is only logged", err: errors.New("x")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			h := Handler(func(context.Context, asynxModels.Event[agg]) error {
				called = true
				return tc.err
			})
			assert.NotPanics(t, func() { h(context.Background(), asynxModels.Event[agg]{EventName: "e"}) })
			assert.True(t, called)
		})
	}
}
