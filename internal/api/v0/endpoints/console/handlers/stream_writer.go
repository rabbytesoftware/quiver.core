package console

type streamWriter struct {
	frames *frameWriter
	name   string
}

func (s *streamWriter) Write(
	p []byte,
) (int, error) {
	s.frames.output(s.name, p)
	return len(p), nil
}
