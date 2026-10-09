package shelf

import "errors"

var ErrNotAWorkdir = errors.New("shelf: not a namespace workdir")

var ErrNotLaunchable = errors.New("shelf: arrow has no launchable desktop entry")
