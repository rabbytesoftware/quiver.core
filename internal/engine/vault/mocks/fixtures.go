package mocks

import (
	"github.com/rabbytesoftware/quiver/internal/domain"
)

func Namespace() domain.Namespace {
	return domain.Namespace("example.com/user/repo")
}

func Arrow() *domain.Arrow {
	return &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "test-arrow", Version: "1.0.0"}}
}

func Quiver() *domain.Collection {
	return &domain.Collection{Meta: domain.CollectionMeta{Name: "test-quiver"}}
}
