package logring

import "time"

type options struct {
	now func() time.Time
}
