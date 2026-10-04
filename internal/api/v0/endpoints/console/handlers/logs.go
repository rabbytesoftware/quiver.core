package console

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/rabbytesoftware/quiver.core/internal/api/libs"
	"github.com/rabbytesoftware/quiver.core/internal/api/middleware"
)

// Logs streams the daemon's log records over a WebSocket.
//
// @Summary      Stream the daemon's logs
// @Description  Upgrades to a WebSocket. The server first replays retained records (the most recent `replay`, or those after `since`), then sends one `ready` frame, then every new record as it is logged. Frames are JSON text frames of type `log`, `ready` or `gap`; see docs/spec/console.md.
// @Description
// @Description  A client that stops reading is not waited on: records it misses are counted and reported in a `gap` frame, and a consumer that stays stalled for 5 seconds is closed with code 1013. Sensitive attribute values are redacted before a record is stored.
// @Tags         console
// @Param        level   query  string  false  "Minimum level: debug, info, warn or error"  default(info)
// @Param        since   query  integer false  "Replay only records with a sequence number above this"
// @Param        replay  query  integer false  "Most records to replay (default 500, at most 2000)"
// @Success      101  "Switching Protocols"
// @Failure      400  {object}  libs.ErrResponse  "level, since or replay is invalid"
// @Router       /console/logs [get]
func (h *Handlers) Logs(c *gin.Context) {
	params, err := parseLogParams(c)
	if err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error(), "")
		return
	}

	conn, err := middleware.Upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	stream := &logStream{conn: conn, ring: h.logs, params: params, stuckLimit: h.stuckLimit}
	stream.run(c.Request.Context())
}
