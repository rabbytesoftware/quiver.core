package strategies

import "net/http"

// Request describes an HTTP request issued through the remote strategy.
// An empty Method is treated as GET.
type Request struct {
	Method  string
	URL     string
	Headers http.Header
	Body    []byte
}

// Response carries the full result of a Request, including non-2xx
// statuses, so callers can distinguish rate limiting from other failures.
type Response struct {
	Status  int
	Headers http.Header
	Body    []byte
}
