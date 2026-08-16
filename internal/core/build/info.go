package build

// Info identifies one running Quiver build and an optional update attempt.
type Info struct {
	Version      string
	BuildID      string
	Digest       string
	AttemptToken string
}
