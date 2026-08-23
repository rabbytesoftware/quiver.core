package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/cli/output"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/flow"
)

// collector records emitted events and can refuse at a chosen call, standing in
// for the consumer going away mid-snapshot when the context is cancelled.
type collector struct {
	got      []flow.Event[output.Watch]
	refuseAt int // 1-based call index that returns false; 0 accepts everything
}

func (c *collector) emit(ev flow.Event[output.Watch]) bool {
	c.got = append(c.got, ev)

	return c.refuseAt == 0 || len(c.got) < c.refuseAt
}

func (c *collector) names() []string {
	names := make([]string, 0, len(c.got))
	for _, ev := range c.got {
		names = append(names, ev.Name)
	}

	return names
}

func TestEmitWatchEvent_ReportsStateThenSteps(t *testing.T) {
	c := &collector{}
	last := ""
	evt := apidto.ArrowRuntimeDTO{
		State: "installing",
		ActiveRun: &apidto.RunRecordDTO{
			Method: "_install",
			Steps: []apidto.StepProgressDTO{
				{Index: 0, Status: "running", Title: "Fetching binary"},
			},
		},
	}

	require.True(t, emitWatchEvent(evt, &last, map[int]string{}, c.emit))
	assert.Equal(t, []string{"installing", "Fetching binary"}, c.names())
	assert.Equal(t, "installing", last)
}

func TestEmitWatchEvent_SkipsUnchangedStateAndSteps(t *testing.T) {
	c := &collector{}
	last := "running"
	seen := map[int]string{0: "running"}
	evt := apidto.ArrowRuntimeDTO{
		State: "running",
		ActiveRun: &apidto.RunRecordDTO{
			Steps: []apidto.StepProgressDTO{{Index: 0, Status: "running", Title: "Fetching binary"}},
		},
	}

	require.True(t, emitWatchEvent(evt, &last, seen, c.emit))
	assert.Empty(t, c.got, "a snapshot that changed nothing should emit nothing")
}

func TestEmitWatchEvent_NoActiveRunEmitsStateOnly(t *testing.T) {
	c := &collector{}
	last := ""
	evt := apidto.ArrowRuntimeDTO{State: "ready"}

	require.True(t, emitWatchEvent(evt, &last, map[int]string{}, c.emit))
	assert.Equal(t, []string{"ready"}, c.names())
}

// Both emit sites must stop the walk when the consumer has gone away, or the
// goroutine keeps translating snapshots nobody will read.
func TestEmitWatchEvent_StopsWhenConsumerRefuses(t *testing.T) {
	testCases := []struct {
		name     string
		refuseAt int
		want     []string
	}{
		{name: "refused on the state transition", refuseAt: 1, want: []string{"installing"}},
		{name: "refused on the first step", refuseAt: 2, want: []string{"installing", "Fetching binary"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c := &collector{refuseAt: tc.refuseAt}
			last := ""
			evt := apidto.ArrowRuntimeDTO{
				State: "installing",
				ActiveRun: &apidto.RunRecordDTO{
					Steps: []apidto.StepProgressDTO{
						{Index: 0, Status: "running", Title: "Fetching binary"},
						{Index: 1, Status: "running", Title: "Extracting"},
					},
				},
			}

			assert.False(t, emitWatchEvent(evt, &last, map[int]string{}, c.emit))
			assert.Equal(t, tc.want, c.names())
		})
	}
}

func TestEmitWatchEvent_UntitledStepGetsPlaceholder(t *testing.T) {
	c := &collector{}
	last := "running"
	evt := apidto.ArrowRuntimeDTO{
		State: "running",
		ActiveRun: &apidto.RunRecordDTO{
			Steps: []apidto.StepProgressDTO{{Index: 0, Status: "running"}},
		},
	}

	require.True(t, emitWatchEvent(evt, &last, map[int]string{}, c.emit))
	require.Len(t, c.got, 1)
	assert.NotEmpty(t, c.got[0].Name, "an untitled step still needs a label to render")
}
