package ownership

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/platform"
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
	) Holder
	Holder(
		path string,
	) (Holder, error)
	Enclosing(
		target string,
	) Holder
	Tag(
		path string,
		bare domain.Namespace,
		workdir string,
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
) Holder {
	tag, err := b.tagger.Read(path)
	if err != nil {
		return Holder{}
	}
	return bundleTagHolder(tag, path)
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
	h := b.Owner(path)
	h.Exists = true
	return h, nil
}

func (b *bundles) Enclosing(
	target string,
) Holder {
	for dir := filepath.Dir(target); dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		if !strings.HasSuffix(dir, platform.BundleExt) {
			continue
		}
		if h := b.Owner(dir); h.Namespace != "" {
			return h
		}
	}
	return Holder{}
}

func (b *bundles) Tag(
	path string,
	bare domain.Namespace,
	workdir string,
	bundle string,
) error {
	return b.tagger.Write(path, BundleTag(bare, workdir, bundle))
}

func BundleTag(
	bare domain.Namespace,
	workdir string,
	bundle string,
) string {
	return bare.String() + "\n" + filepath.Base(bundle) + "\n" + workdir
}

func bundleTagHolder(
	tag string,
	bundle string,
) Holder {
	fields := strings.SplitN(tag, "\n", 3)
	if len(fields) < 2 || fields[1] != filepath.Base(bundle) {
		return Holder{}
	}
	h := Holder{Exists: true, Namespace: domain.Namespace(fields[0])}
	if len(fields) == 3 {
		h.Target = fields[2]
	}
	return h
}
