package picker

type Match string

const (
	MatchExact    Match = "exact"
	MatchAssumed  Match = "assumed"
	MatchEmulated Match = "emulated"
)
