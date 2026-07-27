package providers

import "fmt"

// UnauthorizedError says a host rejected the caller's credentials, which is a
// configuration problem the user can fix, unlike a rate limit that only time
// fixes.
type UnauthorizedError struct {
	Host string
}

func (e *UnauthorizedError) Error() string {
	return fmt.Sprintf("%s rejected the request as unauthorized", e.Host)
}
