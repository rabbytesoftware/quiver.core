package dto

import "github.com/rabbytesoftware/quiver.core/internal/domain"

type InferenceDTO struct {
	Generator  string   `json:"generator" yaml:"generator"`
	Confidence string   `json:"confidence" yaml:"confidence"`
	Warnings   []string `json:"warnings,omitempty" yaml:"warnings,omitempty"`
}

func InferenceDTOFrom(
	g *domain.ArrowGenerator,
) *InferenceDTO {
	if g == nil {
		return nil
	}
	return &InferenceDTO{
		Generator:  g.Name,
		Confidence: g.Confidence,
		Warnings:   g.Warnings,
	}
}
