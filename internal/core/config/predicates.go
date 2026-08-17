package config

import (
	"net"
	"strings"
	"time"
)

const (
	minPort = 1
	maxPort = 65535
)

func validPort(p int) bool {
	return p >= minPort && p <= maxPort
}

func validDuration(s string) bool {
	d, err := time.ParseDuration(s)
	return err == nil && d > 0
}

func validLogLevel(s string) bool {
	switch strings.ToLower(s) {
	case "debug", "trace", "info", "warn", "warning", "error", "fatal", "panic":
		return true
	}

	return false
}

func validHost(s string) bool {
	const sep = "://"

	idx := strings.Index(s, sep)
	if idx < 0 {
		return false
	}

	scheme := s[:idx]
	authority := s[idx+len(sep):]

	if scheme == "unix" {
		return true
	}

	if scheme != "tcp" {
		return false
	}

	_, _, err := net.SplitHostPort(authority)

	return err == nil
}
