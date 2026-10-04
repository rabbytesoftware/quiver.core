package console

import (
	"errors"
	"log/slog"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/rabbytesoftware/quiver.core/internal/core/logring"
)

const (
	defaultReplay = 500
	maxReplay     = 2000
)

type logParams struct {
	level  slog.Level
	since  uint64
	replay int
}

func parseLogParams(
	c *gin.Context,
) (logParams, error) {
	level, err := parseLevelParam(c.Query("level"))
	if err != nil {
		return logParams{}, err
	}
	since, err := parseSinceParam(c.Query("since"))
	if err != nil {
		return logParams{}, err
	}
	replay, err := parseReplayParam(c.Query("replay"))
	if err != nil {
		return logParams{}, err
	}
	return logParams{level: level, since: since, replay: replay}, nil
}

func parseLevelParam(
	raw string,
) (slog.Level, error) {
	if raw == "" {
		return slog.LevelInfo, nil
	}
	if !logring.IsLevel(raw) {
		return 0, errors.New("level must be one of debug, info, warn, error")
	}
	return logring.ParseLevel(raw), nil
}

func parseSinceParam(
	raw string,
) (uint64, error) {
	if raw == "" {
		return 0, nil
	}

	since, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, errors.New("since must be an unsigned integer")
	}
	return since, nil
}

func parseReplayParam(
	raw string,
) (int, error) {
	if raw == "" {
		return defaultReplay, nil
	}

	replay, err := strconv.Atoi(raw)
	if err != nil || replay < 0 {
		return 0, errors.New("replay must be a non-negative integer")
	}
	return min(replay, maxReplay), nil
}
