package command

// LineError reports a command line that cannot be turned into arguments:
// empty, too long, with too many tokens, with control characters or invalid
// UTF-8, or with an unterminated quote or escape.
type LineError struct {
	Reason string
}

func (e *LineError) Error() string {
	return e.Reason
}
