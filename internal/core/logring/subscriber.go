package logring

import "log/slog"

type subscriber struct {
	records chan Record
	level   slog.Level
}

func newSubscriber(
	level slog.Level,
) *subscriber {
	return &subscriber{records: make(chan Record, liveBuffer), level: level}
}

func (s *subscriber) wants(
	rec Record,
) bool {
	return rec.level >= s.level
}

func (s *subscriber) offer(
	rec Record,
) bool {
	select {
	case s.records <- rec:
		return true
	default:
		return false
	}
}
