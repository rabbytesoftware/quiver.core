package dto

// ConsoleExecRequestDTO is the body of a console exec call.
//
// Line is the command to run without the leading quiver, for example
// "install github.com/user/repo". It is split on whitespace, never given to a
// shell, and cannot contain quotes or other shell syntax.
type ConsoleExecRequestDTO struct {
	Line string `json:"line" yaml:"line"`
}
