package command

// Result is how a command finished.
//
// Code follows the CLI's exit codes (0 success, 1 failure, 2 usage, 3 daemon
// unreachable). Err is the message the CLI would print after "quiver:", empty
// on success.
type Result struct {
	Code int
	Err  string
}
