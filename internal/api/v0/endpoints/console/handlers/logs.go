package console

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/rabbytesoftware/quiver.core/internal/api/libs"
	"github.com/rabbytesoftware/quiver.core/internal/api/middleware"
)

// Logs streams the daemon's log records over a WebSocket.
//
// It replays the last 500 records, or those after the since sequence number
// when given, sends one ready frame carrying the newest sequence number (with
// reset set when since was newer than the daemon's newest record, which means
// the daemon restarted), then every record logged afterwards. level is the
// minimum level, defaulting to info. A client that falls 256 records behind is
// disconnected and should reconnect with since set to the last seq it saw. The
// subscription is released as soon as the client leaves.
//
// @Summary      Stream the daemon's logs
// @Description  Upgrades to a WebSocket. Replays the last 500 records (or those after `since`), sends one `ready` frame, then every new record. A client that falls behind is disconnected; reconnect with since=<last seq>.
// @Tags         console
// @Param        level  query  string   false  "Minimum level: debug, info, warn or error"  default(info)
// @Param        since  query  integer  false  "Replay only records with a sequence number above this"
// @Success      101    "Switching Protocols"
// @Failure      400    {object}  libs.ErrResponse  "level or since is invalid"
// @Router       /console/logs [get]
func (h *Handlers) Logs(
	c *gin.Context,
) {
	level, since, ok := readLogQuery(c)
	if !ok {
		return
	}
	conn, err := middleware.Upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	newLogSession(conn, h.logs.Stream(since, level)).run()
}

func readLogQuery(
	c *gin.Context,
) (slog.Level, uint64, bool) {
	level := slog.LevelInfo
	if raw := c.Query("level"); raw != "" && level.UnmarshalText([]byte(raw)) != nil {
		libs.WriteErr(c, http.StatusBadRequest, "level must be debug, info, warn or error", "")
		return level, 0, false
	}
	since, err := strconv.ParseUint(c.DefaultQuery("since", "0"), 10, 64)
	if err != nil {
		libs.WriteErr(c, http.StatusBadRequest, "since must be an unsigned integer", "")
		return level, 0, false
	}
	return level, since, true
}
