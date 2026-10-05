package console

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/api/libs"
	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/clierr"
)

// Commands lists the commands the console may run: the runnable ones that carry
// the console annotation along with every ancestor.
//
// @Summary      List the commands the console may run
// @Description  Returns the runnable commands that carry the console annotation along with all their ancestors.
// @Tags         console
// @Produce      json
// @Success      200  {object}  libs.QueryResponse{data=apidto.ConsoleCommandsDTO}
// @Router       /console/commands [get]
func (h *Handlers) Commands(
	c *gin.Context,
) {
	list := listCommands(h.tree(nil), []apidto.ConsoleCommandDTO{})
	libs.WriteQueryOK(c, apidto.ConsoleCommandsDTO{Commands: list})
}

func listCommands(
	parent *cobra.Command,
	list []apidto.ConsoleCommandDTO,
) []apidto.ConsoleCommandDTO {
	for _, cmd := range parent.Commands() {
		list = appendIfRunnable(list, cmd)
	}
	return list
}

func appendIfRunnable(
	list []apidto.ConsoleCommandDTO,
	cmd *cobra.Command,
) []apidto.ConsoleCommandDTO {
	if !clierr.IsConsole(cmd) {
		return list
	}
	if cmd.Runnable() {
		list = append(list, commandDTO(cmd))
	}
	return listCommands(cmd, list)
}

func commandDTO(
	cmd *cobra.Command,
) apidto.ConsoleCommandDTO {
	return apidto.ConsoleCommandDTO{
		Path:  strings.Fields(cmd.CommandPath())[1:],
		Short: cmd.Short,
		Usage: strings.TrimPrefix(cmd.UseLine(), "quiver "),
	}
}
