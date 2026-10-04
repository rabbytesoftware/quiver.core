// Package command runs the daemon's own CLI command tree on behalf of the
// console, under a strict default-deny policy.
//
// The grammar is not duplicated: every call builds a fresh copy of the tree
// from internal/cli/commands and executes it in-process. What the console may
// reach is decided by the console annotation each command carries next to its
// own definition (see clierr.AllowInConsole), checked on the command and on
// every ancestor.
//
// A line is never given to a shell. Tokenize splits it into arguments with a
// small quote-aware lexer and cobra receives the resulting slice, so command
// substitution, variable expansion, globbing, pipes and redirection have no
// meaning here.
package command

const (
	MaxLineBytes = 1024
	MaxTokens    = 64
)

// New returns an Executor that builds each command tree with opts.
func New(
	opts Options,
) Executor {
	return &treeExecutor{opts: opts}
}
