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

	res, err := lifecycle.Wait(context.Background(), events, "install", nil)
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

	res, err := lifecycle.Wait(context.Background(), events, "install", nil)
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

	res, err := lifecycle.Wait(context.Background(), events, "stop", nil)
	require.NoError(t, err)
	assert.Equal(t, "success", res.Outcome)
}

func TestWait_CallsObserverPerEvent(t *testing.T) {
	events := feed(
		rt("installing", &apidto.RunRecordDTO{Method: "_install"}, nil),
		rt("ready", nil, &apidto.ReturnDTO{Method: "_install", Outcome: "success"}),
	)
	seen := 0
	_, err := lifecycle.Wait(context.Background(), events, "install", func(apidto.ArrowRuntimeDTO) { seen++ })
	require.NoError(t, err)
	assert.Equal(t, 2, seen)
}

func TestWait_ChannelClosedWithoutTerminalErrors(t *testing.T) {
	events := feed(rt("installing", &apidto.RunRecordDTO{Method: "_install"}, nil))

	_, err := lifecycle.Wait(context.Background(), events, "install", nil)
	assert.Error(t, err)
}

func TestWait_ContextTimeout(t *testing.T) {
	ch := make(chan apidto.ArrowRuntimeDTO) // never delivers
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := lifecycle.Wait(ctx, ch, "install", nil)
	assert.Error(t, err)
}

// ─── PlainPrinter ────────────────────────────────────────────────────────────
