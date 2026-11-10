package logged

import (
	"context"
	"errors"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain/auth"
)

type projector struct {
	err     error
	applied []auth.Device
}

func (p *projector) Apply(
	_ context.Context,
	device auth.Device,
) error {
	p.applied = append(p.applied, device)
	return p.err
}

func TestProjection(t *testing.T) {
	boom := errors.New("boom")
	testCases := []struct {
		name    string
		err     error
		wantErr error
	}{
		{name: "success is passed through"},
		{name: "failure is returned", err: boom, wantErr: boom},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p := &projector{err: tc.err}

			err := Projection(p)(context.Background(), asynxModels.Event[auth.Device]{AggregateID: "d1"})

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Len(t, p.applied, 1)
		})
	}
}
