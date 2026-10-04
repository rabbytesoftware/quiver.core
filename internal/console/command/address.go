package command

// Address holds the address the daemon's own commands dial. It is written once
// the daemon's listener is bound and read on every call, which lets the router
// be built before the address is known.
type Address interface {
	// Set records the daemon's own dial URI, for example "unix:///p/quiver.sock".
	Set(
		uri string,
	)

	// Get returns the recorded URI, or empty before Set has been called.
	Get() string
}
