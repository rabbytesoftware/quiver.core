package guard

const MaxEntries = maxEntries

func LimitEntries(
	limit int,
) Option {
	return func(g *Guard) {
		g.maxEntries = limit
	}
}
