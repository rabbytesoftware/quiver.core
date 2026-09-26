package dto

import (
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
)

type ChannelDTO struct {
	Name    string   `json:"name" yaml:"name"`
	Kind    string   `json:"kind" yaml:"kind"`
	Latest  string   `json:"latest,omitempty" yaml:"latest,omitempty"`
	Count   int      `json:"count,omitempty" yaml:"count,omitempty"`
	Members []string `json:"members,omitempty" yaml:"members,omitempty"`
}

type ChannelListDTO struct {
	Channels []ChannelDTO `json:"channels" yaml:"channels"`
}

func ChannelListDTOFrom(
	channels []models.ChannelInfo,
) ChannelListDTO {
	out := make([]ChannelDTO, 0, len(channels))
	for _, c := range channels {
		out = append(out, ChannelDTO{
			Name:    c.Name,
			Kind:    c.Kind,
			Latest:  c.Latest,
			Count:   c.Count,
			Members: c.Members,
		})
	}
	return ChannelListDTO{Channels: out}
}
