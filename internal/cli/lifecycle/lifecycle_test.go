package lifecycle_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/cli/lifecycle"
)

func rt(state string, active *apidto.RunRecordDTO, last *apidto.ReturnDTO) apidto.ArrowRuntimeDTO {
	return apidto.ArrowRuntimeDTO{
		Namespace:  "github.com/user/a",
		State:      state,
		ActiveRun:  active,
		LastReturn: last,
	}
}

func feed(events ...apidto.ArrowRuntimeDTO) <-chan apidto.ArrowRuntimeDTO {
	ch := make(chan apidto.ArrowRuntimeDTO, len(events))
	for _, e := range events {
		ch <- e
	}
	close(ch)
	return ch
}

// ─── MatchesMethod ───────────────────────────────────────────────────────────

func TestMatchesMethod_UnderscoreForm(t *testing.T) {
	assert.True(t, lifecycle.MatchesMethod("_install", "install"))
}

func TestMatchesMethod_RunAliasesExecute(t *testing.T) {
	assert.True(t, lifecycle.MatchesMethod("_execute", "run"))
}

func TestMatchesMethod_CustomExact(t *testing.T) {
	assert.True(t, lifecycle.MatchesMethod("backup", "backup"))
	assert.False(t, lifecycle.MatchesMethod("backup", "restore"))
}

// ─── Wait ────────────────────────────────────────────────────────────────────

func TestWait_ResolvesOnMatchingReturn(t *testing.T) {
	events := feed(
		rt("installing", &apidto.RunRecordDTO{Method: "_install"}, nil),
		rt("ready", nil, &apidto.ReturnDTO{Method: "_install", Outcome: "success"}),
	)

	res, err := lifecycle.Wait(context.Background(), events, "install", nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", res.Outcome)
	assert.Equal(t, "ready", res.State)
}

func TestWait_FailedOutcome(t *testing.T) {
	failMsg := "fetch: 404"
	events := feed(
		rt("installing", &apidto.RunRecordDTO{Method: "_install"}, nil),
		rt("absent", nil, &apidto.ReturnDTO{
			Method:  "_install",
			Outcome: "failed",
			Steps:   []apidto.StepProgressDTO{{Index: 0, Status: "failed", Error: &failMsg, Type: "fetch"}},
		}),
	)

	res, err := lifecycle.Wait(context.Background(), events, "install", nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "failed", res.Outcome)
	require.NotNil(t, res.FailedStep)
	assert.Equal(t, "fetch: 404", *res.FailedStep.Error)
}

func TestWait_IgnoresUnrelatedReturns(t *testing.T) {
	events := feed(
		rt("running", nil, &apidto.ReturnDTO{Method: "_install", Outcome: "success"}), // stale install return
		rt("ready", nil, &apidto.ReturnDTO{Method: "_stop", Outcome: "success"}),
	)

	res, err := lifecycle.Wait(context.Background(), events, "stop", nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", res.Outcome)
}

func TestWait_CallsObserverPerEvent(t *testing.T) {
	events := feed(
		rt("installing", &apidto.RunRecordDTO{Method: "_install"}, nil),
		rt("ready", nil, &apidto.ReturnDTO{Method: "_install", Outcome: "success"}),
	)
	seen := 0
	_, err := lifecycle.Wait(context.Background(), events, "install", nil, func(apidto.ArrowRuntimeDTO) { seen++ })
	require.NoError(t, err)
	assert.Equal(t, 2, seen)
}

func TestWait_ChannelClosedWithoutTerminalErrors(t *testing.T) {
	events := feed(rt("installing", &apidto.RunRecordDTO{Method: "_install"}, nil))

	_, err := lifecycle.Wait(context.Background(), events, "install", nil, nil)
	assert.Error(t, err)
}

func TestWait_ContextTimeout(t *testing.T) {
	ch := make(chan apidto.ArrowRuntimeDTO) // never delivers
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := lifecycle.Wait(ctx, ch, "install", nil, nil)
	assert.Error(t, err)
}

// ─── Wait: the previous run's return ─────────────────────────────────────────

func updateReturn(id, title string) *apidto.ReturnDTO {
	return &apidto.ReturnDTO{
		Method: "_update", Outcome: "success", ExecutionID: id,
		Steps: []apidto.StepProgressDTO{{Index: 0, Status: "completed", Title: title}},
	}
}

func TestWait_PreviousRunsReturn_IsNotThisRunsEnd(t *testing.T) {
	testCases := []struct {
		name      string
		previous  *apidto.ReturnDTO
		events    []apidto.ArrowRuntimeDTO
		wantState string
		wantTitle string
		wantErr   bool
	}{
		{
			name:     "replayed before this run ends",
			previous: updateReturn("e1", "previous"),
			events: []apidto.ArrowRuntimeDTO{
				rt("outdated", nil, updateReturn("e1", "previous")),
				rt("updating", &apidto.RunRecordDTO{Method: "_update"}, updateReturn("e1", "previous")),
				rt("ready", nil, updateReturn("e2", "current")),
			},
			wantState: "ready",
			wantTitle: "current",
		},
		{
			name:     "this run's return arrives without its begin",
			previous: updateReturn("e1", "previous"),
			events: []apidto.ArrowRuntimeDTO{
				rt("outdated", nil, updateReturn("e1", "previous")),
				rt("ready", nil, updateReturn("e2", "current")),
			},
			wantState: "ready",
			wantTitle: "current",
		},
		{
			name:     "only the previous return arrives",
			previous: updateReturn("e1", "previous"),
			events:   []apidto.ArrowRuntimeDTO{rt("outdated", nil, updateReturn("e1", "previous"))},
			wantErr:  true,
		},
		{
			name:     "previous return recorded before returns had ids",
			previous: updateReturn("", "previous"),
			events: []apidto.ArrowRuntimeDTO{
				rt("outdated", nil, updateReturn("", "previous")),
				rt("ready", nil, updateReturn("e2", "current")),
			},
			wantState: "ready",
			wantTitle: "current",
		},
		{
			name:     "a daemon recording no ids: this run's begin tells them apart",
			previous: updateReturn("", "previous"),
			events: []apidto.ArrowRuntimeDTO{
				rt("outdated", nil, updateReturn("", "previous")),
				rt("updating", &apidto.RunRecordDTO{Method: "_update"}, updateReturn("", "previous")),
				rt("ready", nil, updateReturn("", "current")),
			},
			wantState: "ready",
			wantTitle: "current",
		},
		{
			name:     "a daemon recording no ids and no begin seen",
			previous: updateReturn("", "previous"),
			events:   []apidto.ArrowRuntimeDTO{rt("ready", nil, updateReturn("", "current"))},
			wantErr:  true,
		},
		{
			name:     "another method's begin does not count as this run's",
			previous: updateReturn("", "previous"),
			events: []apidto.ArrowRuntimeDTO{
				rt("stopping", &apidto.RunRecordDTO{Method: "_stop"}, updateReturn("", "previous")),
				rt("outdated", nil, updateReturn("", "previous")),
			},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := lifecycle.Wait(context.Background(), feed(tc.events...), "update", tc.previous, nil)

			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantState, res.State)
			require.Len(t, res.Steps, 1)
			assert.Equal(t, tc.wantTitle, res.Steps[0].Title)
		})
	}
}

// ─── PlainPrinter ────────────────────────────────────────────────────────────
