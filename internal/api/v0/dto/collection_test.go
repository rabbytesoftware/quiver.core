package dto_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestQuiverDTOFrom(t *testing.T) {
	q := domain.Collection{
		Namespace: "github.com/user/repo",
	}
	d := dto.QuiverDTOFrom(q)
	assert.Equal(t, "github.com/user/repo", d.Namespace)
}
