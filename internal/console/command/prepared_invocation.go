package command

import (
	"context"
	"io"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/clierr"
)

type preparedInvocation struct {
	root *cobra.Command
}

func (p *preparedInvocation) Run(
	ctx context.Context,
	stdout io.Writer,
	stderr io.Writer,
) Result {
	p.root.SetOut(stdout)
	p.root.SetErr(stderr)

	done := make(chan Result, 1)
	go func() { done <- p.execute(ctx) }()

	select {
	case result := <-done:
		return result
	case <-ctx.Done():
		return Result{Code: 1, Err: ctx.Err().Error()}
	}
}

func (p *preparedInvocation) execute(
	ctx context.Context,
) (result Result) {
	defer recoverResult(&result)

	err := p.root.ExecuteContext(ctx)
	if err == nil {
		return Result{}
	}
	return Result{Code: clierr.ExitCode(err), Err: err.Error()}
}

func recoverResult(
	result *Result,
) {
	recovered := recover()
	if recovered == nil {
		return
	}

	slog.Error("console command panicked", "component", "console", "panic", recovered)
	*result = Result{Code: 1, Err: "internal error"}
}
