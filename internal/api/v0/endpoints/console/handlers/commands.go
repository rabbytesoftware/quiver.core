package console

import (
	"github.com/gin-gonic/gin"

	"github.com/rabbytesoftware/quiver.core/internal/api/libs"
	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/console/command"
)

// Commands lists the commands the console may run.
//
// @Summary      List the commands the console may run
// @Description  Returns every command, with its usage and flags, that carries the console annotation and whose parent chain does too. Clients render help and completion from this list instead of keeping their own.
// @Tags         console
// @Produce      json
// @Success      200  {object}  libs.QueryResponse{data=apidto.ConsoleCommandsDTO}
// @Router       /console/commands [get]
func (h *Handlers) Commands(c *gin.Context) {
	libs.WriteQueryOK(c, commandsDTO(h.exec.Commands()))
}

func commandsDTO(
	infos []command.CommandInfo,
) apidto.ConsoleCommandsDTO {
	commands := make([]apidto.ConsoleCommandDTO, 0, len(infos))
	for _, info := range infos {
		commands = append(commands, commandDTO(info))
	}
	return apidto.ConsoleCommandsDTO{Commands: commands}
}

func commandDTO(
	info command.CommandInfo,
) apidto.ConsoleCommandDTO {
	flags := make([]apidto.ConsoleFlagDTO, 0, len(info.Flags))
	for _, flag := range info.Flags {
		flags = append(flags, apidto.ConsoleFlagDTO{
			Name:       flag.Name,
			Shorthand:  flag.Shorthand,
			Usage:      flag.Usage,
			TakesValue: flag.Takes,
		})
	}

	return apidto.ConsoleCommandDTO{
		Path:    append([]string{}, info.Path...),
		Short:   info.Short,
		Usage:   info.Usage,
		Aliases: append([]string{}, info.Aliases...),
		Flags:   flags,
	}
}
