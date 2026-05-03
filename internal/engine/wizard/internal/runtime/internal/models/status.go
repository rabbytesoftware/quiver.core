package models

type Status string

const (
	StatusPrepared Status = "prepared"
	StatusRunning  Status = "running"
	StatusStopping Status = "stopping"
	StatusKilling  Status = "killing"
	StatusFinished Status = "finished"
)

func (s Status) String() string { return string(s) }

func (s Status) IsActive() bool {
	return s == StatusRunning || s == StatusStopping || s == StatusKilling
}

func (s Status) IsFinished() bool { return s == StatusFinished }
