package mocks

import (
	"github.com/rabbytesoftware/quiver/internal/domain"
)

func Namespace() domain.Namespace {
	return domain.Namespace("example.com/user/repo")
}

func ArrowManifest() *domain.ArrowManifest {
	return &domain.ArrowManifest{Name: "test-arrow", Version: "1.0.0"}
}

func QuiverManifest() *domain.QuiverManifest {
	return &domain.QuiverManifest{Name: "test-quiver"}
}
