package models

type PathStatus struct {
	BinDir     string
	OnPath     bool
	Configured bool
	Files      []string
}
