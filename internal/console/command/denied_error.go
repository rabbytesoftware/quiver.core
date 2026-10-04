package command

// DeniedError reports a command line the console refuses to run. Command is
// the command path involved when one could be resolved.
type DeniedError struct {
	Command string
	Reason  string
}

func (e *DeniedError) Error() string {
	return e.Reason
}
