package dto

type ExposeRefusalDTO struct {
	Kind   string `json:"kind" yaml:"kind"`
	Name   string `json:"name" yaml:"name"`
	Reason string `json:"reason" yaml:"reason"`
}
