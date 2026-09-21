//go:build windows

package main

// surviveBrokenLogPipe is a no-op on Windows, which has no SIGPIPE: a write to
// a pipe whose reader has gone returns ERROR_BROKEN_PIPE to the caller and
// never raises a signal, so the daemon already survives losing its log reader
// there. The unix build's doc comment explains what it guards against and why
// only the daemon wants it.
func surviveBrokenLogPipe() func() {
	return func() {}
}
