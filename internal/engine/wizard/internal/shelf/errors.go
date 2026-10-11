package shelf

import "errors"

var ErrNotAWorkdir = errors.New("shelf: not a namespace workdir")

var ErrNoApp = errors.New("shelf: no app to start was found for this arrow")
