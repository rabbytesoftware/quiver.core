package command

// Executor validates and runs console command lines.
type Executor interface {
	// Prepare authorizes tokens against a freshly built command tree and, on
	// success, returns an Invocation that will run exactly the command that
	// was authorized.
	//
	// bearer is the caller's own bearer token, or empty on a trusted local
	// socket. Commands call back into this daemon with it, so they run with
	// the caller's authority and no more.
	//
	// It returns a *DeniedError when the command is not reachable from the
	// console, and ErrUnavailable when the daemon's address is not known yet.
	Prepare(
		tokens []string,
		bearer string,
	) (Invocation, error)

	// Commands describes every command the console may run, for help and
	// completion.
	Commands() []CommandInfo
}
