package exposerinternal

import (
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf"
)

func From(
	applied shelf.Applied,
) *domainRuntime.ExposeResult {
	if len(applied.Entries) == 0 && len(applied.Refused) == 0 {
		return nil
	}
	result := &domainRuntime.ExposeResult{}
	for _, entry := range applied.Entries {
		result.Entries = append(result.Entries, domainRuntime.ExposedEntry{
			Kind:     entry.Kind,
			Name:     entry.Name,
			Target:   entry.Target,
			Location: entry.Location,
		})
	}
	for _, refusal := range applied.Refused {
		result.Refused = append(result.Refused, domainRuntime.ExposeRefusal{
			Kind:   refusal.Kind,
			Name:   refusal.Name,
			Reason: refusal.Reason,
		})
	}
	return result
}
