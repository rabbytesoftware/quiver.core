package providers

import (
	"bytes"
	"net/url"
	"path"
	"regexp"
	"strings"

	"golang.org/x/net/html"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func parseExpandedAssets(
	body []byte,
	pageURL string,
) ([]domain.ReleaseAsset, error) {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, ErrUnexpectedPage
	}

	list := findFirstList(doc)
	if list == nil {
		return nil, ErrUnexpectedPage
	}

	base, err := url.Parse(pageURL)
	if err != nil {
		return nil, ErrUnexpectedPage
	}

	digest := regexp.MustCompile(`sha256:[0-9a-f]{64}`)
	assets := make([]domain.ReleaseAsset, 0)
	for _, item := range findListItems(list) {
		asset, ok := assetFromItem(item, base, digest)
		if ok {
			assets = append(assets, asset)
		}
	}
	return assets, nil
}

func findFirstList(
	n *html.Node,
) *html.Node {
	if n.Type == html.ElementNode && n.Data == "ul" {
		return n
	}

	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if found := findFirstList(child); found != nil {
			return found
		}
	}
	return nil
}

func findListItems(
	list *html.Node,
) []*html.Node {
	items := make([]*html.Node, 0)
	for child := list.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && child.Data == "li" {
			items = append(items, child)
		}
	}
	return items
}

func assetFromItem(
	item *html.Node,
	base *url.URL,
	digestPattern *regexp.Regexp,
) (domain.ReleaseAsset, bool) {
	href, text := firstAssetLink(item)
	if href == "" || strings.Contains(href, "/archive/") {
		return domain.ReleaseAsset{}, false
	}

	name := fileName(href)
	label := ""
	if text != name {
		label = text
	}

	digest := ""
	if matches := digestPattern.FindAllString(textContent(item), -1); len(matches) > 0 {
		digest = matches[len(matches)-1]
	}

	return domain.ReleaseAsset{
		Name:   name,
		Label:  label,
		URL:    resolveHref(base, href),
		Digest: digest,
	}, true
}

func fileName(
	href string,
) string {
	name := path.Base(href)
	if unescaped, err := url.PathUnescape(name); err == nil {
		return unescaped
	}
	return name
}

func resolveHref(
	base *url.URL,
	href string,
) string {
	ref, err := url.Parse(href)
	if err != nil {
		return href
	}
	return base.ResolveReference(ref).String()
}

func firstAssetLink(
	n *html.Node,
) (string, string) {
	if n.Type == html.ElementNode && n.Data == "a" {
		href := attrValue(n, "href")
		if href != "" {
			return href, strings.TrimSpace(textContent(n))
		}
	}

	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if href, text := firstAssetLink(child); href != "" {
			return href, text
		}
	}
	return "", ""
}

func attrValue(
	n *html.Node,
	key string,
) string {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

func textContent(
	n *html.Node,
) string {
	var buf strings.Builder
	writeTextContent(n, &buf)
	return buf.String()
}

func writeTextContent(
	n *html.Node,
	buf *strings.Builder,
) {
	if n.Type == html.TextNode {
		buf.WriteString(n.Data)
	}

	for child := n.FirstChild; child != nil; child = child.NextSibling {
		writeTextContent(child, buf)
	}
}
