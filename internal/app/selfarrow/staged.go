package selfarrow

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"regexp"
	"strconv"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/core/fns"
	"github.com/rabbytesoftware/quiver.core/internal/core/selfupdate"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

// StagedOutcome is what booting with a staged binary came to.
type StagedOutcome string

const (
	// StagedNone: nothing was staged, or the decision belongs to someone else.
	StagedNone StagedOutcome = "none"
	// StagedApplied: the staged binary is the build now running.
	StagedApplied StagedOutcome = "applied"
	// StagedHandedOver: a newer verified binary was staged; the daemon is
	// handing over to it.
	StagedHandedOver StagedOutcome = "handed_over"
	// StagedKept: a newer verified binary is staged and this process cannot
	// hand over to it.
	StagedKept StagedOutcome = "kept"
	// StagedDiscarded: the staged binary was unusable or older than the
	// running build, and is gone.
	StagedDiscarded StagedOutcome = "discarded"
	// StagedHandoverFailed: a handover to the staged binary was started and
	// this build is still the one running.
	StagedHandoverFailed StagedOutcome = "handover_failed"
)

// Running identifies the build doing the reconciling.
type Running struct {
	Version    string
	Commit     string
	Executable string
}

// Handover hands the daemon over to a binary: it records path and asks the
// daemon to shut down and relaunch into it.
type Handover interface {
	Fire(path string)
}

// stagedRuntime is the subset of the runtime repository ReconcileStaged needs.
type stagedRuntime interface {
	GetRuntime(
		ctx context.Context,
		ns domain.Namespace,
	) (*domainRuntime.ArrowRuntime, error)
	MarkActivating(
		ctx context.Context,
		ns domain.Namespace,
	) error
	ClearPendingActivation(
		ctx context.Context,
		ns domain.Namespace,
	) error
}

// ReconcileStaged settles the binary an update staged for ns's runtime and
// nobody applied, as a booting daemon finds it: the user quit before asking
// for the restart, or the handover they asked for never took.
//
// A binary that is already the running build is simply forgotten. One a
// handover was started for, and that did not take, is discarded rather than
// tried again, as is one that no longer matches the size and SHA-256 it was
// staged with or that is older than this build. Anything else is newer and
// verified: the attempt is recorded before the daemon hands over, so a
// handover that fails is never repeated by the next boot.
//
// trig may be nil, for a process that cannot hand itself over.
func ReconcileStaged(
	ctx context.Context,
	rt stagedRuntime,
	trig Handover,
	ns domain.Namespace,
	running Running,
) (StagedOutcome, error) {
	current, err := rt.GetRuntime(ctx, ns)
	if err != nil {
		return StagedNone, fmt.Errorf("selfarrow: reconcile staged: read runtime of %s: %w", ns, err)
	}
	if current == nil || current.PendingActivation == nil {
		return StagedNone, nil
	}
	pending := current.PendingActivation

	if isRunning(pending, running) {
		return StagedApplied, forget(ctx, rt, ns, pending, false)
	}
	if pending.Activating {
		return StagedHandoverFailed, forget(ctx, rt, ns, pending, true)
	}
	if err := selfupdate.Verify(ctx, pending.Path, pending.Size, pending.Digest); err != nil {
		slog.WarnContext(ctx, "selfarrow: staged binary discarded", "ns", ns, "err", err)
		return StagedDiscarded, forget(ctx, rt, ns, pending, true)
	}
	if olderThan(pending.Version, running.Version) {
		slog.WarnContext(ctx, "selfarrow: staged binary is older than the running build, discarded",
			"ns", ns, "staged", pending.Version, "running", running.Version)
		return StagedDiscarded, forget(ctx, rt, ns, pending, true)
	}
	if trig == nil {
		return StagedKept, nil
	}

	if err := rt.MarkActivating(ctx, ns); err != nil {
		if errors.Is(err, apperrors.ErrStateViolation) {
			return StagedNone, nil
		}
		return StagedNone, fmt.Errorf("selfarrow: reconcile staged: %w", err)
	}
	trig.Fire(pending.Path)
	return StagedHandedOver, nil
}

// isRunning reports whether pending is the build now running: the process was
// started from its file, or it carries the version and commit running reports.
func isRunning(
	pending *domainRuntime.PendingActivation,
	running Running,
) bool {
	if running.Executable != "" && sameFile(running.Executable, pending.Path) {
		return true
	}
	return pending.Version == running.Version && (pending.Commit == "" || pending.Commit == running.Commit)
}

// forget drops the record of pending and, when removeFile, its file.
func forget(
	ctx context.Context,
	rt stagedRuntime,
	ns domain.Namespace,
	pending *domainRuntime.PendingActivation,
	removeFile bool,
) error {
	if removeFile {
		if err := fns.Remove(ctx, pending.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			slog.WarnContext(ctx, "selfarrow: remove a staged binary", "path", pending.Path, "err", err)
		}
	}
	if err := rt.ClearPendingActivation(ctx, ns); err != nil {
		return fmt.Errorf("selfarrow: reconcile staged: %w", err)
	}
	return nil
}

var digitRun = regexp.MustCompile(`\d+`)

// olderThan reports whether staged is a lower version than running, comparing
// the numbers in them in order (beta-26.5-4 is 26, 5, 4). Versions without
// numbers, such as a rolling tag, cannot be ordered and are never older.
func olderThan(
	staged string,
	running string,
) bool {
	a, b := numbersIn(staged), numbersIn(running)
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	for i := 0; i < max(len(a), len(b)); i++ {
		x, y := at(a, i), at(b, i)
		if x != y {
			return x < y
		}
	}
	return false
}

func numbersIn(
	version string,
) []int {
	var numbers []int
	for _, run := range digitRun.FindAllString(version, -1) {
		n, err := strconv.Atoi(run)
		if err != nil {
			return nil
		}
		numbers = append(numbers, n)
	}
	return numbers
}

func at(
	numbers []int,
	i int,
) int {
	if i >= len(numbers) {
		return 0
	}
	return numbers[i]
}
