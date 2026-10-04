package command

import "sync/atomic"

type atomicAddress struct {
	value atomic.Value
}

// NewAddress returns an empty Address safe for concurrent use.
func NewAddress() Address {
	return &atomicAddress{}
}

func (a *atomicAddress) Set(
	uri string,
) {
	a.value.Store(uri)
}

func (a *atomicAddress) Get() string {
	uri, _ := a.value.Load().(string)
	return uri
}
