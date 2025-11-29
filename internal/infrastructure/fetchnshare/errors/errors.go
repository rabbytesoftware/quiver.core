package errors

import "fmt"

type FNSError struct {
	Op   string
	Path string
	Err  error
}

func (e *FNSError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("%s %s: %v", e.Op, e.Path, e.Err)
	}
	return fmt.Sprintf("%s: %v", e.Op, e.Err)
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

