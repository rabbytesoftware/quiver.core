//go:build windows

package transports_test

import "sync/atomic"

var testPipeSeq atomic.Int64
