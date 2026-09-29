package models

const (
	BundleExt   = ".app"
	AppImageExt = ".AppImage"
	ExeExt      = ".exe"
)

type Candidate struct {
	Name     string
	Display  string
	Target   string
	Icon     string
	Depth    int
	Declared bool
}

func (c Candidate) DisplayName() string {
	if c.Display != "" {
		return c.Display
	}
	return c.Name
}

type Placement struct {
	Location string
	Refused  string
}
