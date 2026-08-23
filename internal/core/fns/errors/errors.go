package errors

import "fmt"

type FNSError struct {
	Op   string
	Path string
	Err  error
}

func (e *FNSError) Error() string {
	// Collapse nested layers that repeat the same path (e.g. a Download that
	// wraps a GetInfo probe on the same URL) so the path and the intermediate
	// operation name aren't echoed at every level.
	inner := e.Err
	for {
		fe, ok := inner.(*FNSError)
		if !ok || fe.Path != e.Path {
			break
		}
		inner = fe.Err
	}

	if e.Path != "" {
		return fmt.Sprintf("%s %s: %v", e.Op, e.Path, inner)
	}
	return fmt.Sprintf("%s: %v", e.Op, inner)
}

func (e *FNSError) Unwrap() error {
	return e.Err
}

func Op(operation, path string, err error) error {
	if err == nil {
		return nil
	}
	return &FNSError{
		Op:   operation,
		Path: path,
		Err:  err,
	}
}

func NotFound(operation, path string) error {
	return &FNSError{
		Op:   operation,
		Path: path,
		Err:  fmt.Errorf("not found"),
	}
}

func Unsupported(operation, resource string) error {
	return &FNSError{
		Op:  operation,
		Err: fmt.Errorf("not supported for %s", resource),
	}
}
