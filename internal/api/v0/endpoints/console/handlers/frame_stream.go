package console

type frameStream struct {
	frames *frames
	name   string
}

// Write sends data as one out frame of this stream and always reports it as
// fully written, so a command is never aborted by its own output. Output past
// the 256 KiB limit is cut once with a single truncation note and every later
// write is discarded.
func (s *frameStream) Write(
	data []byte,
) (int, error) {
	s.frames.accept(s.name, data)
	return len(data), nil
}
