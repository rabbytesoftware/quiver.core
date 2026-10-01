package tui

import (
	"errors"
	"io"
	"syscall"
)

// outputWriter remembers the first error its destination returned. An
// encoder may report a failed write as text of its own, as yaml.v3 does, and
// only the original error says whether the reader simply went away.
type outputWriter struct {
	w   io.Writer
	err error
}

func (o *outputWriter) Write(p []byte) (int, error) {
	n, err := o.w.Write(p)
	if err != nil && o.err == nil {
		o.err = err
	}
	return n, err
}

// cause is the destination's own error when a write failed, else err.
func (o *outputWriter) cause(err error) error {
	if o.err != nil {
		return o.err
	}
	return err
}

// readerGone reports a write that failed only because whatever read the
// output stopped reading.
func readerGone(err error) bool {
	return errors.Is(err, io.ErrClosedPipe) ||
		errors.Is(err, syscall.EPIPE) ||
		isPlatformBrokenPipe(err)
}
