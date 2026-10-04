package command

import "errors"

// ErrUnavailable is returned by Prepare when the daemon's own address is not
// known, so commands have nothing to dial.
var ErrUnavailable = errors.New("console: daemon address is not available")
