package ownership

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/platform"
)

type Tagger interface {
	Read(
		path string,
	) (string, error)
	Write(
		path string,
		value string,
	) error
}

type Bundles interface {
	Owner(
		path string,
	) domain.Namespace
	Holder(
		path string,
	) (Holder, error)
	Enclosing(
		target string,
	) domain.Namespace
	Tag(
		path string,
		bare domain.Namespace,
		bundle string,
	) error
}

type bundles struct {
	tagger Tagger
}

func NewBundles(
	tagger Tagger,
) Bundles {
	return &bundles{tagger: tagger}
}

func (b *bundles) Owner(
	path string,
) domain.Namespace {
	tag, err := b.tagger.Read(path)
	if err != nil {
		return ""
	}
	return bundleTagOwner(tag, path)
}

func (b *bundles) Holder(
	path string,
) (Holder, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Holder{}, nil
	}
	if err != nil {
		return Holder{}, fmt.Errorf("inspect %s: %w", path, err)
	}
	if !info.IsDir() {
		return Holder{Exists: true}, nil
	}
	return Holder{Exists: true, Namespace: b.Owner(path)}, nil
}

func (b *bundles) Enclosing(
	target string,
) domain.Namespace {
	for dir := filepath.Dir(target); dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		if !strings.HasSuffix(dir, platform.BundleExt) {
			continue
		}
		if owner := b.Owner(dir); owner != "" {
			return owner
		}
	}
	return ""
}

func (b *bundles) Tag(
	path string,
	bare domain.Namespace,
	bundle string,
) error {
	return b.tagger.Write(path, BundleTag(bare, bundle))
}

func BundleTag(
	bare domain.Namespace,
	bundle string,
) string {
	return bare.String() + "\n" + filepath.Base(bundle)
}

func bundleTagOwner(
	tag string,
	bundle string,
) domain.Namespace {
	owner, name, ok := strings.Cut(tag, "\n")
	if !ok || name != filepath.Base(bundle) {
		return ""
	}
	return domain.Namespace(owner)
}
