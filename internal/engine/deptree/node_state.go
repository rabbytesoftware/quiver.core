package deptree

type nodeState uint8

const (
	unvisited  nodeState = iota
	inProgress nodeState = iota
	done       nodeState = iota
)
