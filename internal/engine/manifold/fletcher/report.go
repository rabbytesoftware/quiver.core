package fletcher

const heuristics = "fletcher/1"

type Report struct {
	Heuristics string
	Confidence Confidence
	Warnings   []string
}
