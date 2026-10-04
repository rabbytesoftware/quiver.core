package models

// ShutdownInfo identifies the daemon process that was asked to stop, so a
// caller can wait for it to die and start the same command again.
type ShutdownInfo struct {
	PID  int
	Exe  string
	Args []string
}
