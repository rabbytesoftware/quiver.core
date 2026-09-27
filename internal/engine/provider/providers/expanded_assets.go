package providers

import (
	"bytes"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

var (
	digestPattern = regexp.MustCompile(`sha256:[0-9a-f]{64}`)
	sizePattern   = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?)\s*(Bytes|KB|MB|GB|TB)$`)
)

func parseExpandedAssets(
	body []byte,
	pageURL string,
) ([]Asset, error) {
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

	assets := make([]Asset, 0)
	for _, item := range findListItems(list) {
		asset, ok := assetFromItem(item, base)
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
) (Asset, bool) {
	href, text := firstAssetLink(item)
	if href == "" || strings.Contains(href, "/archive/") {
		return Asset{}, false
	}

	name := path.Base(href)
	if text != "" {
		name = text
	}

	digest := ""
	size := int64(0)
	for _, line := range itemTextLines(item) {
		if match := digestPattern.FindString(line); match != "" {
			digest = match
		}
		if match := sizePattern.FindStringSubmatch(line); match != nil {
			size = parseSize(match[1], match[2])
		}
	}

	return Asset{
		Name:   name,
		URL:    resolveHref(base, href),
		Size:   size,
		Digest: digest,
	}, true
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

func itemTextLines(
	n *html.Node,
) []string {
	var lines []string
	collectTextLines(n, &lines)
	return lines
}

func collectTextLines(
	n *html.Node,
	lines *[]string,
) {
	if n.Type == html.TextNode {
		trimmed := strings.TrimSpace(n.Data)
		if trimmed != "" {
			*lines = append(*lines, trimmed)
		}
	}

	for child := n.FirstChild; child != nil; child = child.NextSibling {
		collectTextLines(child, lines)
	}
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

func parseSize(
	number string,
	unit string,
) int64 {
	value, err := strconv.ParseFloat(number, 64)
	if err != nil {
		return 0
	}
	return int64(value * float64(sizeMultiplier(unit)))
}

func sizeMultiplier(
	unit string,
) int64 {
	switch unit {
	case "Bytes":
		return 1
	case "KB":
		return 1 << 10
	case "MB":
		return 1 << 20
	case "GB":
		return 1 << 30
	case "TB":
		return 1 << 40
	}
	return 0
}
