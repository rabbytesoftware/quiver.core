package domain

type ExposeKind string

const (
	ExposeKindCLI     ExposeKind = "cli"
	ExposeKindDesktop ExposeKind = "desktop"
)
