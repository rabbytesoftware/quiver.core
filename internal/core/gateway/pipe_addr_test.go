package gateway_test

type pipeAddr string

func (a pipeAddr) Network() string {
	return "pipe"
}

func (a pipeAddr) String() string {
	return string(a)
}
