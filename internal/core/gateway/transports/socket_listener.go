package transports

import (
	"net"
	"os"
)

type socketListener struct {
	net.Listener
	path      string
	boundInfo os.FileInfo
}

// Close closes the listener and removes the socket file at path -- but only
// when that path still names the exact file this listener bound. A second
// daemon that raced this one for the same path and lost the bind can still
// end up owning that name later (net.Listen unlinks-then-binds, so a loser
// that lands its own Listen call in between wins the name even though this
// listener is the one still accepting connections); blindly unlinking here
// would then delete a sibling's live socket out from under it -- one still
// listening but now unreachable through the path everyone else dials,
// indistinguishable from a crash to anything that only checks the path.
// os.SameFile (inode identity on Unix, file index on Windows) is the only
// check that survives the name being reused in between Listen and Close:
// Stat alone succeeds identically whether it is this listener's file or a
// completely different one now sitting at the same path.
func (l *socketListener) Close() error {
	err := l.Listener.Close()
	if current, statErr := os.Stat(l.path); statErr == nil && os.SameFile(l.boundInfo, current) {
		_ = os.Remove(l.path)
	}
	return err
}
