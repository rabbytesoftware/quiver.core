package console

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/api/libs"
	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/clierr"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/session"
	"github.com/rabbytesoftware/quiver.core/internal/core/logring"
)

const maxRunning = 4

type Handlers struct {
	logs  *logring.Ring
	slots chan struct{}
	root  func(session.Session) *cobra.Command
}

// New returns Handlers that stream logs and run the daemon's own CLI tree,
// which reports version as its own.
func New(logs *logring.Ring, version string) *Handlers {
	return &Handlers{
		logs:  logs,
		slots: make(chan struct{}, maxRunning),
		root: func(sess session.Session) *cobra.Command {
			root := &cobra.Command{Use: "quiver", SilenceUsage: true, SilenceErrors: true}
			commands.New(commands.Deps{Version: version, Session: sess}).Attach(root)
			_ = root.PersistentFlags().Set("output", "table")
			return root
		},
	}
}

// @Summary      List the commands the console may run
// @Description  Returns the runnable commands that carry the console annotation along with all their ancestors.
// @Tags         console
// @Produce      json
// @Success      200  {object}  libs.QueryResponse{data=apidto.ConsoleCommandsDTO}
// @Router       /console/commands [get]
func (h *Handlers) Commands(c *gin.Context) {
	libs.WriteQueryOK(c, apidto.ConsoleCommandsDTO{Commands: collect(h.root(nil), []apidto.ConsoleCommandDTO{})})
}

func collect(parent *cobra.Command, list []apidto.ConsoleCommandDTO) []apidto.ConsoleCommandDTO {
	for _, cmd := range parent.Commands() {
		if !clierr.IsConsole(cmd) {
			continue
		}
		if cmd.Runnable() {
			list = append(list, apidto.ConsoleCommandDTO{
				Path:  strings.Fields(cmd.CommandPath())[1:],
				Short: cmd.Short,
				Usage: strings.TrimPrefix(cmd.UseLine(), "quiver "),
			})
		}
		list = collect(cmd, list)
	}
	return list
}
