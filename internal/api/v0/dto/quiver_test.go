package dto_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

func TestQuiverDTOFrom(t *testing.T) {
	q := domain.Quiver{
		Namespace: "github.com/user/repo",
		Manifest:  domain.QuiverManifest{Name: "My Quiver", Tags: []string{"tag1"}},
	}
	d := dto.QuiverDTOFrom(q)
	assert.Equal(t, "github.com/user/repo", d.Namespace)
	assert.Equal(t, "My Quiver", d.Name)
	assert.Equal(t, []string{"tag1"}, d.Tags)
}
