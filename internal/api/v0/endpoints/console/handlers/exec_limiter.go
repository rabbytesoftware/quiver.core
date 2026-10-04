package console

// ExecLimiter bounds how many commands run at once.
type ExecLimiter interface {
	// Acquire reserves a slot for device. It returns false when the device,
	// or the daemon as a whole, is at its limit. release frees the slot and is
	// safe to call more than once.
	Acquire(
		device string,
	) (release func(), ok bool)
}
