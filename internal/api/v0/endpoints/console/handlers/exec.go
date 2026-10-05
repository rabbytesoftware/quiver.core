package console

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/api/libs"
	"github.com/rabbytesoftware/quiver.core/internal/api/middleware"
	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/clierr"
	"github.com/rabbytesoftware/quiver.core/internal/domain/auth"
)

const (
	execTimeout    = 10 * time.Minute
	outputLimit    = 256 * 1024
	maxBody        = 4096
	localDevice    = "local"
	truncationNote = "\n[output truncated at 256 KiB]\n"
)

// Exec runs one console command and streams its output.
//
// The line is split on whitespace and never reaches a shell. It runs only when
// the command it names, and every ancestor below the root, carry the console
// annotation; anything else is refused with 403 naming it. A line that is empty,
// over 1024 bytes or holds control or shell characters is a 400, and more than
// four commands at once is a 429. Otherwise the response is NDJSON: one out
// frame per write to stdout or stderr, then exactly one exit frame with the
// exit code and error text. The command runs with empty input, so confirmations
// answer no, for at most ten minutes or until the client disconnects, and its
// output is cut at 256 KiB. Each call logs one audit line naming the device and
// the resolved command path, never the raw line.
//
// @Summary      Run a console command
// @Description  Runs a line against the daemon's own CLI tree and streams newline-delimited JSON: `out` frames, then exactly one `exit` frame. Only commands marked for the console run (403 otherwise). The line is split on spaces and never reaches a shell; quoting is not supported. At most 4 commands run at once (429), each limited to 10 minutes and 256 KiB of output.
// @Tags         console
// @Accept       json
// @Produce      application/x-ndjson
// @Param        body  body  apidto.ConsoleExecRequestDTO  true  "The command line, without the leading quiver"
// @Success      200   "Stream of out and exit frames"
// @Failure      400   {object}  libs.ErrResponse  "The line is empty, too long or has forbidden characters"
// @Failure      403   {object}  libs.ErrResponse  "The command is not available in the console"
// @Failure      429   {object}  libs.ErrResponse  "Too many commands are running"
// @Router       /console/exec [post]
func (h *Handlers) Exec(
	c *gin.Context,
) {
	args, ok := readLine(c)
	if !ok {
		return
	}
	root := h.tree(newOwnSession(c))
	cmd, err := allowed(root, args)
	if err != nil {
		slog.WarnContext(c.Request.Context(), "exec denied", "component", "console", "device", device(c), "command", cmd.CommandPath())
		libs.WriteErr(c, http.StatusForbidden, err.Error(), "")
		return
	}
	if !h.acquire() {
		libs.WriteErr(c, http.StatusTooManyRequests, "too many console commands are running", "")
		return
	}
	defer h.release()

	stream(c, root, cmd, args)
}

func readLine(
	c *gin.Context,
) ([]string, bool) {
	var req apidto.ConsoleExecRequestDTO
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBody)
	if err := c.ShouldBindJSON(&req); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, `request body must be {"line": "<command>"}`, "")
		return nil, false
	}
	args, err := parseLine(req.Line)
	if err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error(), "")
		return nil, false
	}
	return args, true
}

func stream(
	c *gin.Context,
	root *cobra.Command,
	cmd *cobra.Command,
	args []string,
) {
	c.Header("Content-Type", "application/x-ndjson")
	c.Writer.Flush()

	ctx, cancel := context.WithTimeout(c.Request.Context(), execTimeout)
	defer cancel()
	out := &frames{writer: c.Writer}
	root.SetArgs(args)
	root.SetIn(strings.NewReader(""))
	root.SetOut(out.stream("stdout"))
	root.SetErr(out.stream("stderr"))

	start := time.Now()
	code, message := execute(ctx, root)
	out.exit(code, message)
	slog.InfoContext(c.Request.Context(), "exec", "component", "console", "device", device(c), "command", cmd.CommandPath(), "code", code, "took", time.Since(start))
}

func execute(
	ctx context.Context,
	root *cobra.Command,
) (code int, message string) {
	defer recoverAsExit(ctx, &code, &message)

	err := root.ExecuteContext(ctx)
	if err != nil {
		return clierr.ExitCode(err), err.Error()
	}
	return 0, ""
}

func recoverAsExit(
	ctx context.Context,
	code *int,
	message *string,
) {
	recovered := recover()
	if recovered == nil {
		return
	}
	slog.ErrorContext(ctx, "console command panicked", "component", "console", "panic", recovered)
	*code, *message = 1, "internal error"
}

func device(
	c *gin.Context,
) string {
	value, found := c.Get(middleware.DeviceContextKey)
	if !found {
		return localDevice
	}
	dev, ok := value.(auth.Device)
	if !ok || dev.ID == "" {
		return localDevice
	}
	return dev.ID
}
