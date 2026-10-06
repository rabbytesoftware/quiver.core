//go:build darwin || freebsd || openbsd || netbsd || dragonfly

package surface

// MaxSocketPath is the longest socket address a bind accepts here: sockaddr_un
// holds 104 bytes on these systems, the last of them the NUL. Measured on
// macOS: a bind at 103 bytes works and one at 104 fails.
const MaxSocketPath = 103
