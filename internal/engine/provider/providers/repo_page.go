package providers

import (
	"bytes"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

const generatedSocialImageHost = "opengraph.githubassets.com"

type RepoPage struct {
	Description       string
	SocialImage       string
	CustomSocialImage bool
	OwnerIsOrg        bool
	OwnerAvatar       string
}

func parseRepoPage(
	body []byte,
	slug string,
) (RepoPage, error) {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return RepoPage{}, ErrUnexpectedPage
	}

	meta := collectMetaTags(doc)
	description, hasDescription := meta["og:description"]
	image, hasImage := meta["og:image"]
	if !hasDescription && !hasImage {
		return RepoPage{}, ErrUnexpectedPage
	}

	return RepoPage{
		Description:       cleanDescription(description, slug),
		SocialImage:       image,
		CustomSocialImage: image != "" && !isGeneratedSocialImage(image),
	}, nil
}

func cleanDescription(
	description string,
	slug string,
) string {
	trimmed := trimSuffixFold(strings.TrimSpace(description), " - "+slug)
	trimmed = trimSuffixFold(trimmed, "Contribute to "+slug+" development by creating an account on GitHub.")
	return strings.TrimSpace(trimmed)
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

func collectMetaTags(
	n *html.Node,
) map[string]string {
	found := make(map[string]string)
	collectMetaTagsInto(n, found)
	return found
}

func collectMetaTagsInto(
	n *html.Node,
	found map[string]string,
) {
	if n.Type == html.ElementNode && n.Data == "meta" {
		property, content, ok := metaProperty(n)
		if ok {
			found[property] = content
		}
	}

	for child := n.FirstChild; child != nil; child = child.NextSibling {
		collectMetaTagsInto(child, found)
	}
}

func metaProperty(
	n *html.Node,
) (string, string, bool) {
	var property, content string
	for _, attr := range n.Attr {
		if attr.Key == "property" {
			property = attr.Val
		}
		if attr.Key == "content" {
			content = attr.Val
		}
	}
	return property, content, property != ""
}

func isGeneratedSocialImage(
	rawURL string,
) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return parsed.Host == generatedSocialImageHost
}
