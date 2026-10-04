package console

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/api/libs"
	"github.com/rabbytesoftware/quiver.core/internal/api/middleware"
	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/clierr"
	"github.com/rabbytesoftware/quiver.core/internal/core/gateway"
	"github.com/rabbytesoftware/quiver.core/internal/domain/auth"
)

const (
	execTimeout = 10 * time.Minute
	outputLimit = 256 * 1024
)

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
func (h *Handlers) Exec(c *gin.Context) {
	var req apidto.ConsoleExecRequestDTO
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if err := c.ShouldBindJSON(&req); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, `request body must be {"line": "<command>"}`, "")
		return
	}
	args, err := parseLine(req.Line)
	if err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error(), "")
		return
	}

	addr, _ := c.Request.Context().Value(http.LocalAddrContextKey).(net.Addr)
	token, _ := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
	root := h.root(ownSession{uri: gateway.DialURI(addr), token: strings.TrimSpace(token)})
	cmd, err := allowed(root, args)
	if err != nil {
		slog.WarnContext(c.Request.Context(), "exec denied", "component", "console", "device", device(c), "command", cmd.CommandPath())
		libs.WriteErr(c, http.StatusForbidden, err.Error(), "")
		return
	}

	select {
	case h.slots <- struct{}{}:
		defer func() { <-h.slots }()
	default:
		libs.WriteErr(c, http.StatusTooManyRequests, "too many console commands are running", "")
		return
	}

	c.Header("Content-Type", "application/x-ndjson")
	c.Writer.Flush()

	ctx, cancel := context.WithTimeout(c.Request.Context(), execTimeout)
	defer cancel()
	out := &frames{w: c.Writer}
	root.SetArgs(args)
	root.SetIn(strings.NewReader(""))
	root.SetOut(out.stream("stdout"))
	root.SetErr(out.stream("stderr"))

	start := time.Now()
	code, msg := run(ctx, root)
	out.mu.Lock()
	out.write(map[string]any{"type": "exit", "code": code, "error": msg})
	out.mu.Unlock()
	slog.InfoContext(c.Request.Context(), "exec", "component", "console", "device", device(c), "command", cmd.CommandPath(), "code", code, "took", time.Since(start))
}

func run(ctx context.Context, root *cobra.Command) (code int, msg string) {
	defer func() {
		if r := recover(); r != nil {
			slog.ErrorContext(ctx, "console command panicked", "component", "console", "panic", r)
			code, msg = 1, "internal error"
		}
	}()

	if err := root.ExecuteContext(ctx); err != nil {
		return clierr.ExitCode(err), err.Error()
	}
	return 0, ""
}

func device(c *gin.Context) string {
	if v, ok := c.Get(middleware.DeviceContextKey); ok {
		if d, ok := v.(auth.Device); ok {
			return d.ID
		}
	}
	return "local"
}

// frames writes the NDJSON stream, discarding output past the limit.
type frames struct {
	mu   sync.Mutex
	w    gin.ResponseWriter
	seen int
}

type stream struct {
	frames *frames
	name   string
}

func (f *frames) stream(name string) io.Writer { return stream{f, name} }

// Write emits p as one out frame and never fails, so a command is not aborted
// by its own output. Past the limit it adds one truncation note, then drops.
func (s stream) Write(p []byte) (int, error) {
	f := s.frames
	f.mu.Lock()
	defer f.mu.Unlock()

	before := f.seen
	f.seen += len(p)
	if before >= outputLimit {
		return len(p), nil
	}
	keep := min(len(p), outputLimit-before)
	f.write(map[string]any{"type": "out", "stream": s.name, "data": string(p[:keep])})
	if keep < len(p) {
		f.write(map[string]any{"type": "out", "stream": "stderr", "data": "\n[output truncated at 256 KiB]\n"})
	}
	return len(p), nil
}

func (f *frames) write(frame any) {
	line, _ := json.Marshal(frame)
	_, _ = f.w.Write(append(line, '\n'))
	f.w.Flush()
}
