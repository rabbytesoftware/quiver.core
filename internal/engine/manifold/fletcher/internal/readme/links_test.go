package readme

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func githubBase(
	owner string,
	repo string,
	ref string,
) RawBase {
	return RawBase{
		Raw:  "https://raw.githubusercontent.com/" + owner + "/" + repo + "/" + ref + "/{file}",
		Blob: "https://github.com/" + owner + "/" + repo + "/blob/" + ref + "/{file}",
	}
}

func TestImages_ExtractsMarkdownAndHTMLSources(
	t *testing.T,
) {
	raw := []byte("![a](one.png)\n<img src=\"two.png\">\n```\n![c](inside-fence.png)\n```\n")

	images := Images(raw)

	a := assert.New(t)
	a.Len(images, 2)
	a.Equal("one.png", images[0].Src)
	a.Equal(0, images[0].Line)
	a.Equal("two.png", images[1].Src)
	a.Equal(1, images[1].Line)
}

func TestImages_EmptyForNoImages(
	t *testing.T,
) {
	images := Images([]byte("just prose, no pictures here"))

	assert.Empty(t, images)
}
