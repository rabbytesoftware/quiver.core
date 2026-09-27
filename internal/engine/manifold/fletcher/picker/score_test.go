package picker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScore_Classification_Values(t *testing.T) {
	testCases := []struct {
		name  string
		class classification
		want  int
	}{
		{name: "archive", class: classification{format: FormatArchive}, want: 30},
		{name: "binary", class: classification{format: FormatBinary}, want: 30},
		{name: "appimage", class: classification{format: FormatAppImage}, want: 30},
		{name: "dmg", class: classification{format: FormatDMG}, want: 20},
		{name: "musl bonus", class: classification{format: FormatArchive, musl: true}, want: 32},
		{name: "unknown format", class: classification{format: Format("other")}, want: 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, score(tc.class))
		})
	}
}

func TestScore_TieRank_Order(t *testing.T) {
	testCases := []struct {
		name  string
		class classification
		want  int
	}{
		{name: "archive", class: classification{asset: asset("a.zip"), format: FormatArchive}, want: archiveRank},
		{name: "appimage", class: classification{asset: asset("a.AppImage"), format: FormatAppImage}, want: appImageRank},
		{name: "bare binary", class: classification{asset: asset("a-linux"), format: FormatBinary}, want: binaryRank},
		{name: "exe", class: classification{asset: asset("a.exe"), format: FormatBinary, exe: true}, want: exeRank},
		{name: "dmg", class: classification{asset: asset("a.dmg"), format: FormatDMG}, want: dmgRank},
		{name: "unknown", class: classification{asset: asset("a"), format: Format("other")}, want: unknownRank},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tieRank(tc.class))
		})
	}
	assert.Less(t, archiveRank, appImageRank)
	assert.Less(t, appImageRank, binaryRank)
	assert.Less(t, binaryRank, exeRank)
	assert.Less(t, exeRank, dmgRank)
}
