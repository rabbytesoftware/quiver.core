package unpack

import "errors"

type GuardOption func(*Guard)

func SkipEscapingLinks() GuardOption {
	return func(g *Guard) {
		g.skipEscaping = true
	}
}

func (g *Guard) tolerateEscape(
	err error,
) error {
	if g.skipEscaping && errors.Is(err, ErrEscape) {
		return nil
	}

	return err
}
