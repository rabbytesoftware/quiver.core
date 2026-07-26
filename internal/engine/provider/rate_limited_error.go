package provider

import (
	"fmt"
	"time"
)

// RateLimitedError says a host refused the search because the caller is out of
// budget. It is deliberately a distinct type: telling the user "GitHub is rate
// limited, try in 40s" is not the same as showing them an empty result list.
// A zero RetryAfter means the host gave no usable hint.
type RateLimitedError struct {
	Host       string
	RetryAfter time.Duration
}

func (e *RateLimitedError) Error() string {
	if e.RetryAfter <= 0 {
		return fmt.Sprintf("%s is rate limited", e.Host)
	}
	return fmt.Sprintf("%s is rate limited, retry in %s", e.Host, e.RetryAfter)
}
