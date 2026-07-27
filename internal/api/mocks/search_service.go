package mocks

import (
	"context"

	"github.com/rabbytesoftware/quiver.core/internal/app/models"
)

type SearchService struct {
	SearchResult []models.SearchResult
	SearchErr    error
	SearchQuery  models.SearchQuery
	SearchCalls  int
}

func (m *SearchService) Search(
	_ context.Context,
	q models.SearchQuery,
) ([]models.SearchResult, error) {
	m.SearchCalls++
	m.SearchQuery = q
	return m.SearchResult, m.SearchErr
}
