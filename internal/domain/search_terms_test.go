package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestSearchTerms(t *testing.T) {
	cases := map[string][]string{
		"quiver-chat":     {"quiver", "chat"},
		"quiver.chat":     {"quiver", "chat"},
		"quiver_chat":     {"quiver", "chat"},
		"  quiver  chat ": {"quiver", "chat"},
		"Quiver.Chat":     {"Quiver", "Chat"},
		"---":             {},
		"bruno":           {"bruno"},
	}
	for text, want := range cases {
		assert.Equal(t, want, domain.SearchTerms(text), text)
	}
}
