//go:build !windows

package tui

func isPlatformBrokenPipe(error) bool { return false }
