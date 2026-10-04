package console

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/rabbytesoftware/quiver.core/internal/api/libs"
	"github.com/rabbytesoftware/quiver.core/internal/api/middleware"
)

// @Summary      Stream the daemon's logs
// @Description  Upgrades to a WebSocket. Replays the last 500 records (or those after `since`), sends one `ready` frame, then every new record. A client that falls behind is disconnected; reconnect with since=<last seq>.
// @Tags         console
// @Param        level  query  string   false  "Minimum level: debug, info, warn or error"  default(info)
// @Param        since  query  integer  false  "Replay only records with a sequence number above this"
// @Success      101    "Switching Protocols"
// @Failure      400    {object}  libs.ErrResponse  "level or since is invalid"
// @Router       /console/logs [get]
func (h *Handlers) Logs(c *gin.Context) {
	level := slog.LevelInfo
	if q := c.Query("level"); q != "" && level.UnmarshalText([]byte(q)) != nil {
		libs.WriteErr(c, http.StatusBadRequest, "level must be debug, info, warn or error", "")
		return
	}
	since, err := strconv.ParseUint(c.DefaultQuery("since", "0"), 10, 64)
	if err != nil {
		libs.WriteErr(c, http.StatusBadRequest, "since must be an unsigned integer", "")
		return
	}
	conn, err := middleware.Upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()

	stream := h.logs.Stream(since, level)
	defer stream.Close()
	closed := watch(conn)

	for _, rec := range stream.Replay {
		if write(conn, rec) != nil {
			return
		}
	}
	ready := map[string]any{"type": "ready", "seq": stream.Seq}
	if stream.Reset {
		ready["reset"] = true
	}
	if write(conn, ready) != nil {
		return
	}
	for {
		select {
		case rec, ok := <-stream.Live:
			if !ok || write(conn, rec) != nil {
				return
			}
		case <-closed:
			return
		}
	}
}

func write(conn *websocket.Conn, frame any) error {
	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return conn.WriteJSON(frame)
}

// watch returns a channel closed once the client goes away; clients send nothing.
func watch(conn *websocket.Conn) <-chan struct{} {
	closed := make(chan struct{})
	conn.SetReadLimit(1024)
	go func() {
		defer close(closed)
		for {
			if _, _, err := conn.NextReader(); err != nil {
				return
			}
		}
	}()
	return closed
}
