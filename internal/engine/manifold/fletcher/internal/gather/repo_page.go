package gather

import (
	"bytes"
	"context"
	"net/url"
	"strings"
	"unicode"

	"golang.org/x/net/html"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/hosts"
)

type repoPage struct {
	description string
	socialImage string
}

func (d *drafter) repoPage(
	ctx context.Context,
	host hosts.Host,
	ns domain.Namespace,
) (repoPage, error) {
	pageURL := host.RepoPageURL(ns)
	if pageURL == "" {
		return repoPage{}, nil
	}
	body, err := d.fetch.prefix(ctx, pageURL, maxPageBytes)
	if err != nil {
		return repoPage{}, err
	}
	owner, repo := coordinates(ns)
	return parsePage(body, pageURL, owner+"/"+repo), nil
}

func parsePage(
	body []byte,
	pageURL string,
	slug string,
) repoPage {
	meta := openGraph(body)
	return repoPage{
		description: cleanDescription(meta["og:description"], slug),
		socialImage: authoredImage(meta["og:image"], pageURL, slug),
	}
}

func openGraph(
	body []byte,
) map[string]string {
	found := make(map[string]string)
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return found
	}
	collectMeta(doc, found)
	return found
}

func collectMeta(
	n *html.Node,
	found map[string]string,
) {
	if n.Type == html.ElementNode && n.Data == "meta" {
		property, content := metaAttrs(n)
		if _, seen := found[property]; property != "" && !seen {
			found[property] = content
		}
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		collectMeta(child, found)
	}
}

func metaAttrs(
	n *html.Node,
) (string, string) {
	var property, content string
	for _, attr := range n.Attr {
		switch attr.Key {
		case "property":
			property = attr.Val
		case "content":
			content = attr.Val
		}
	}
	return property, content
}

func cleanDescription(
	description string,
	slug string,
) string {
	text := strings.TrimSpace(trimSuffixFold(strings.TrimSpace(description), " - "+slug))
	if !containsFold(text, slug) {
		return text
	}
	kept := make([]string, 0)
	for _, sentence := range sentences(text) {
		if !containsFold(sentence, slug) {
			kept = append(kept, strings.TrimSpace(sentence))
		}
	}
	return strings.TrimSpace(strings.Join(kept, " "))
}

func sentences(
	text string,
) []string {
	var out []string
	start := 0
	for i := 0; i+1 < len(text); i++ {
		if strings.IndexByte(".!?", text[i]) >= 0 && unicode.IsSpace(rune(text[i+1])) {
			out = append(out, text[start:i+1])
			start = i + 1
		}
	}
	return append(out, text[start:])
}

func authoredImage(
	image string,
	pageURL string,
	slug string,
) string {
	if image == "" || containsFold(image, slug) {
		return ""
	}
	base, err := url.Parse(pageURL)
	if err != nil {
		return ""
	}
	ref, err := url.Parse(image)
	if err != nil {
		return ""
	}
	return base.ResolveReference(ref).String()
}

func trimSuffixFold(
	s string,
	suffix string,
) string {
	if len(s) < len(suffix) || !strings.EqualFold(s[len(s)-len(suffix):], suffix) {
		return s
	}
	return s[:len(s)-len(suffix)]
}

func containsFold(
	s string,
	substr string,
) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
