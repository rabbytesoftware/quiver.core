//go:build integration

package kit

import (
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type ForgeRepo struct {
	Description string
	Readme      string
	Binary      string
	Asset       string
	Dir         string
	Tags        []string
}

func (r ForgeRepo) assetName(
	tag string,
	platform domain.OS,
) string {
	return expandAsset(r.Asset, tag, platform)
}

func (r ForgeRepo) entryName(
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

func (r ForgeRepo) script(
	tag string,
) []byte {
	return []byte("#!/bin/sh\necho " + r.Binary + " " + tag + "\n")
}

func (r ForgeRepo) latest() string {
	if len(r.Tags) == 0 {
		return ""
	}
	return r.Tags[len(r.Tags)-1]
}
