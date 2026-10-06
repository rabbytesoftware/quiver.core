//go:build !(darwin || freebsd || openbsd || netbsd || dragonfly)

package surface

// MaxSocketPath is the longest socket address a bind accepts here: sockaddr_un
// holds 108 bytes on linux and windows, the last of them the NUL. Measured on
// Windows 11: a bind and dial at 107 bytes work and a bind at 108 fails.
const MaxSocketPath = 107
