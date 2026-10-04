package console

import "sync"

type slotLimiter struct {
	mu        sync.Mutex
	perDevice int
	global    int
	total     int
	held      map[string]int
}

// NewExecLimiter returns an ExecLimiter allowing perDevice concurrent commands
// for each device and global across all devices.
func NewExecLimiter(
	perDevice int,
	global int,
) ExecLimiter {
	return &slotLimiter{perDevice: perDevice, global: global, held: make(map[string]int)}
}

func (l *slotLimiter) Acquire(
	device string,
) (func(), bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.total >= l.global || l.held[device] >= l.perDevice {
		return nil, false
	}

	l.held[device]++
	l.total++

	var once sync.Once
	return func() { once.Do(func() { l.release(device) }) }, true
}

func (l *slotLimiter) release(
	device string,
) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.total--
	l.held[device]--
	if l.held[device] == 0 {
		delete(l.held, device)
	}
}
