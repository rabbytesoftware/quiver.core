package dto_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

func TestExposeResultDTOFrom(t *testing.T) {
	testCases := []struct {
		name    string
		exposed *domainRuntime.ExposeResult
		want    *dto.ExposeResultDTO
	}{
		{name: "nil exposed stays nil", exposed: nil, want: nil},
		{
			name: "entries and refusals are mapped",
			exposed: &domainRuntime.ExposeResult{
				Entries: []domainRuntime.ExposedEntry{{
					Kind:     domain.ExposeKindCLI,
					Name:     "tool",
					Target:   "/w/bin/tool",
					Location: "/b/tool",
				}},
				Refused: []domainRuntime.ExposeRefusal{{
					Kind:   domain.ExposeKindDesktop,
					Name:   "App",
					Reason: "target not found",
				}},
			},
			want: &dto.ExposeResultDTO{
				Entries: []dto.ExposedEntryDTO{{Kind: "cli", Name: "tool", Target: "/w/bin/tool", Location: "/b/tool"}},
				Refused: []dto.ExposeRefusalDTO{{Kind: "desktop", Name: "App", Reason: "target not found"}},
			},
		},
		{
			name:    "empty result keeps empty slices",
			exposed: &domainRuntime.ExposeResult{},
			want:    &dto.ExposeResultDTO{Entries: []dto.ExposedEntryDTO{}, Refused: []dto.ExposeRefusalDTO{}},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, dto.ExposeResultDTOFrom(tc.exposed))
		})
	}
}
