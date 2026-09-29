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
	Dir         string
	Tags        []string
}

func (r HostRepo) assetName(
	tag string,
	platform domain.OS,
) string {
	return expandAsset(r.Asset, tag, platform)
}

func (r HostRepo) entryName(
	tag string,
	platform domain.OS,
) string {
	if r.Dir == "" {
		return r.Binary
	}
	return expandAsset(r.Dir, tag, platform) + "/" + r.Binary
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
