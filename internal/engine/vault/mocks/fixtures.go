package mocks

import (
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func Namespace() domain.Namespace {
	return domain.Namespace("example.com/user/repo")
}

func Arrow() *domain.Arrow {
	return &domain.Arrow{
		Namespace: Namespace().WithRef("v1.0.0"),
		ArrowMeta: domain.ArrowMeta{Name: "test-arrow"},
	}
}

func Quiver() *domain.Collection {
	return &domain.Collection{Meta: domain.CollectionMeta{Name: "test-quiver"}}
}
