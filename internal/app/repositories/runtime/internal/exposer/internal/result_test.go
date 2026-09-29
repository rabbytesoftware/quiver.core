package exposerinternal_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	exposerinternal "github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal/exposer/internal"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf"
)

func TestFrom(t *testing.T) {
	testCases := []struct {
		name    string
		applied shelf.Applied
		want    *domainRuntime.ExposeResult
	}{
		{name: "empty applied is nil"},
		{
			name: "entries and refusals are mapped",
			applied: shelf.Applied{
				Entries: []shelf.AppliedEntry{{Kind: domain.ExposeKindCLI, Name: "tool", Target: "/w/bin/tool", Location: "/q/bin/tool"}},
				Refused: []shelf.Refusal{{Kind: domain.ExposeKindDesktop, Name: "App", Reason: "target not found"}},
			},
			want: &domainRuntime.ExposeResult{
				Entries: []domainRuntime.ExposedEntry{{Kind: domain.ExposeKindCLI, Name: "tool", Target: "/w/bin/tool", Location: "/q/bin/tool"}},
				Refused: []domainRuntime.ExposeRefusal{{Kind: domain.ExposeKindDesktop, Name: "App", Reason: "target not found"}},
			},
		},
		{
			name:    "refusals alone are kept",
			applied: shelf.Applied{Refused: []shelf.Refusal{{Kind: domain.ExposeKindCLI, Name: "tool", Reason: "exists"}}},
			want: &domainRuntime.ExposeResult{
				Refused: []domainRuntime.ExposeRefusal{{Kind: domain.ExposeKindCLI, Name: "tool", Reason: "exists"}},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, exposerinternal.From(tc.applied))
		})
	}
}
