package bracket

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestUpdateTargets_UndoOnlyRemovesItsOwnEntry(t *testing.T) {
	first := domain.Available{Ref: "r", Commit: "c1"}
	second := domain.Available{Ref: "r", Commit: "c2"}

	testCases := []struct {
		name   string
		act    func(targets Targets)
		want   domain.Available
		wantOK bool
	}{
		{
			name: "undo restores what the put replaced",
			act: func(targets Targets) {
				targets.Put(rollingRow, first)
				targets.Put(rollingRow, second)()
			},
			want:   first,
			wantOK: true,
		},
		{
			name: "undo on an empty slot leaves it empty",
			act: func(targets Targets) {
				targets.Put(rollingRow, first)()
			},
		},
		{
			name: "undo after a newer put leaves the newer entry",
			act: func(targets Targets) {
				undo := targets.Put(rollingRow, first)
				targets.Put(rollingRow, second)
				undo()
			},
			want:   second,
			wantOK: true,
		},
		{
			name: "undo after the entry was taken changes nothing",
			act: func(targets Targets) {
				undo := targets.Put(rollingRow, first)
				targets.Take(rollingRow)
				undo()
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			targets := NewTargets()

			tc.act(targets)

			got, ok := targets.Take(rollingRow)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}
