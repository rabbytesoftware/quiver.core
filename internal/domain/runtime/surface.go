package runtime

// SurfaceMode says where an arrow's interface is served from.
type SurfaceMode string

const (
	// SurfaceModeListen: the arrow serves on a daemon-allocated unix socket.
	SurfaceModeListen SurfaceMode = "listen"
	// SurfaceModeStatic: the daemon serves files from Dir.
	SurfaceModeStatic SurfaceMode = "static"
)

// Surface is the interface an execution has opened. It lives on Execution so
// it shares the execution's lifetime: ending or replacing the execution
// closes the surface with no extra bookkeeping.
type Surface struct {
	Mode SurfaceMode `json:"mode"`
	// Path is the initial path the shell opens, always starting with "/".
	Path string `json:"path"`
	// Dir is the absolute directory served in static mode.
	Dir string `json:"dir,omitempty"`
	// Ready flips true once the surface answers requests.
	Ready bool `json:"ready"`
}
