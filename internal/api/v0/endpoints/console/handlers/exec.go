package console

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/rabbytesoftware/quiver.core/internal/api/libs"
	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/console/command"
	"github.com/rabbytesoftware/quiver.core/internal/core/logring"
)

const maxExecBody = 4096

// Exec runs one console command line and streams its output.
//
// @Summary      Run a console command
// @Description  Runs a command line against the daemon's own CLI command tree and streams newline-delimited JSON frames: `out` frames carrying stdout or stderr text, then exactly one `exit` frame with the exit code.
// @Description
// @Description  The line is split into arguments by a quote-aware tokenizer and never given to a shell. Only commands that opt in to the console are reachable (403 otherwise), the redirecting --server, --context and --config flags are refused, and confirmations never answer yes: destructive commands need --yes. Commands run with the caller's own credentials.
// @Description
// @Description  Limits: lines up to 1024 bytes and 64 arguments, 256 KiB of output per call, a 10 minute timeout, 2 concurrent commands per device and 8 overall.
// @Tags         console
// @Accept       json
// @Produce      application/x-ndjson
// @Param        body  body  apidto.ConsoleExecRequestDTO  true  "The command line, without the leading quiver"
// @Success      200   "Stream of out and exit frames"
// @Failure      400   {object}  libs.ErrResponse  "The line is empty, too long, or cannot be tokenized"
// @Failure      403   {object}  libs.ErrResponse  "The command is not available in the console"
// @Failure      429   {object}  libs.ErrResponse  "Too many commands are already running"
// @Failure      503   {object}  libs.ErrResponse  "The daemon's own address is not known yet"
// @Router       /console/exec [post]
func (h *Handlers) Exec(c *gin.Context) {
	line, ok := h.readLine(c)
	if !ok {
		return
	}

	tokens, err := command.Tokenize(line)
	if err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error(), "")
		return
	}

	device := deviceID(c)
	invocation, ok := h.prepare(c, device, line, tokens)
	if !ok {
		return
	}

	release, ok := h.limiter.Acquire(device)
	if !ok {
		libs.WriteErr(c, http.StatusTooManyRequests, "too many console commands are already running", "")
		return
	}
	defer release()

	h.stream(c, device, line, invocation)
}

func (h *Handlers) readLine(
	c *gin.Context,
) (string, bool) {
	body := http.MaxBytesReader(c.Writer, c.Request.Body, maxExecBody)
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()

	var req apidto.ConsoleExecRequestDTO
	if err := decoder.Decode(&req); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, `request body must be {"line": "<command>"}`, "")
		return "", false
	}
	return req.Line, true
}

func (h *Handlers) prepare(
	c *gin.Context,
	device string,
	line string,
	tokens []string,
) (command.Invocation, bool) {
	invocation, err := h.exec.Prepare(tokens, bearerOf(c))
	if err == nil {
		return invocation, true
	}

	h.reject(c, device, line, err)
	return nil, false
}

func (h *Handlers) reject(
	c *gin.Context,
	device string,
	line string,
	err error,
) {
	var denied *command.DeniedError
	if errors.As(err, &denied) {
		slog.WarnContext(c.Request.Context(), "exec denied", "component", "console", "device", device, "line", logring.RedactLine(line), "reason", denied.Reason)
		libs.WriteErr(c, http.StatusForbidden, denied.Reason, "")
		return
	}
	if errors.Is(err, command.ErrUnavailable) {
		libs.WriteErr(c, http.StatusServiceUnavailable, "the console cannot reach this daemon's own address", "")
		return
	}
	libs.WriteErr(c, http.StatusInternalServerError, "internal error", "", err)
}

func (h *Handlers) stream(
	c *gin.Context,
	device string,
	line string,
	invocation command.Invocation,
) {
	c.Header("Content-Type", "application/x-ndjson")
	c.Header("Cache-Control", "no-store")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	c.Writer.WriteHeaderNow()
	c.Writer.Flush()

	ctx, cancel := context.WithTimeout(c.Request.Context(), h.execTimeout)
	defer cancel()

	frames := newFrameWriter(c.Writer, h.outputLimit)
	start := time.Now()
	result := invocation.Run(ctx, frames.stream("stdout"), frames.stream("stderr"))
	result = h.explainTimeout(ctx, result)
	frames.exit(result)

	slog.InfoContext(c.Request.Context(), "exec",
		"component", "console",
		"device", device,
		"line", logring.RedactLine(line),
		"code", result.Code,
		"took", time.Since(start),
	)
}

func (h *Handlers) explainTimeout(
	ctx context.Context,
	result command.Result,
) command.Result {
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return result
	}
	return command.Result{Code: 1, Err: fmt.Sprintf("timed out after %s", h.execTimeout)}
}
