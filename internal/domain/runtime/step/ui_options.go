package step

// UIOptions is the interface a run step opens for as long as its process
// lives. Exactly one of Listen or Static is set (enforced by the run_ui rule,
// not here), and an empty Listen with an empty Static means the default
// listen on unix.
type UIOptions struct {
	// Title is the name the shell shows for the interface.
	Title string `json:"title,omitempty"`
	// Path is the initial path, default "/".
	Path string `json:"path,omitempty"`
	// Listen lists the transports the arrow can serve on ("unix", "pipe").
	Listen []string `json:"listen,omitempty"`
	// Static is a directory, relative to the install path, the daemon serves.
	Static string `json:"static,omitempty"`
}

// Listens reports whether the arrow serves the interface itself on a socket
// the daemon provisions, as opposed to the daemon serving a static folder.
func (o UIOptions) Listens() bool {
	return o.Static == ""
}
