package manifold

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher"
)

func TestWithFletcher_ReachesEveryConstructor(t *testing.T) {
	testCases := []struct {
		name  string
		build func(fl fletcher.Fletcher) Manifold
	}{
		{
			name: "New",
			build: func(fl fletcher.Fletcher) Manifold {
				return New(time.Second, nil, time.Hour, WithFletcher(fl))
			},
		},
		{
			name: "NewWithClock",
			build: func(fl fletcher.Fletcher) Manifold {
				return NewWithClock(time.Second, nil, time.Hour, time.Now, WithFletcher(fl))
			},
		},
		{
			name: "NewWithResolvers",
			build: func(fl fletcher.Fletcher) Manifold {
				return NewWithResolvers(&stubResolver{}, &stubConstraintResolver{}, nil, WithFletcher(fl))
			},
		},
		{
			name: "NewWithResolversAndClock",
			build: func(fl fletcher.Fletcher) Manifold {
				return NewWithResolversAndClock(&stubResolver{}, &stubConstraintResolver{}, nil, time.Now, WithFletcher(fl))
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fl := &stubFletcher{answer: draftAt(t, "v1.2.0")}

			_, _, err := tc.build(fl).ProbeArrow(context.Background(), domain.Namespace("github.com/acme/tool@v1.2.0"), fletcher.Hint{})

			require.NoError(t, err)
			assert.Equal(t, []string{"v1.2.0"}, fl.probeTags)
		})
	}
}

func TestWithFletcher_Nil_LeavesFletcherDisabled(t *testing.T) {
	m := NewWithResolvers(&stubResolver{}, &stubConstraintResolver{}, nil, WithFletcher(nil))

	_, _, err := m.ProbeArrow(context.Background(), domain.Namespace("github.com/acme/tool@v1.2.0"), fletcher.Hint{})

	assert.ErrorIs(t, err, fletcher.ErrNotFletchable)
}
