//go:build integration

package kit

import (
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type HostRepo struct {
	Description string
	Readme      string
	Binary      string
	Asset       string
	Tags        []string

	APIDescription string
	AvatarURL      string
	Files          map[string][]byte
}

func (r HostRepo) assetName(
	tag string,
	platform domain.OS,
) string {
	return expandAsset(r.Asset, tag, platform)
}

func expandAsset(
	pattern string,
	tag string,
	platform domain.OS,
) string {
	goos, goarch, _ := strings.Cut(platform.String(), "/")
	return strings.NewReplacer("{tag}", tag, "{os}", goos, "{arch}", goarch).Replace(pattern)
}

func (r HostRepo) script(
	tag string,
) []byte {
	return []byte("#!/bin/sh\necho " + r.Binary + " " + tag + "\n")
}
