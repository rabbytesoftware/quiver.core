package picker

const (
	portableScore = 30
	dmgScore      = 20
	muslBonus     = 2
)

const (
	archiveRank = iota
	appImageRank
	binaryRank
	exeRank
	dmgRank
	unknownRank
)

func score(
	c classification,
) int {
	base := formatScore(c.format)
	if c.musl {
		return base + muslBonus
	}
	return base
}

func formatScore(
	format Format,
) int {
	switch format {
	case FormatArchive, FormatBinary, FormatAppImage:
		return portableScore
	case FormatDMG:
		return dmgScore
	}
	return 0
}

func tieRank(
	c classification,
) int {
	switch c.format {
	case FormatArchive:
		return archiveRank
	case FormatAppImage:
		return appImageRank
	case FormatBinary:
		return binaryRankOf(c)
	case FormatDMG:
		return dmgRank
	}
	return unknownRank
}

func binaryRankOf(
	c classification,
) int {
	if c.exe {
		return exeRank
	}
	return binaryRank
}
