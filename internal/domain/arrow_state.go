package domain

type ArrowState string

const (
	ArrowStateAbsent       ArrowState = "absent"
	ArrowStateInstalling   ArrowState = "installing"
	ArrowStateReady        ArrowState = "ready"
	ArrowStateRunning      ArrowState = "running"
	ArrowStateStopping     ArrowState = "stopping"
	ArrowStateUninstalling ArrowState = "uninstalling"
	ArrowStateRemoved      ArrowState = "removed"
)
