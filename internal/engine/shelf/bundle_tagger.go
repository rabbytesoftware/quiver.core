package shelf

type bundleTagger interface {
	read(
		path string,
	) (string, error)
	write(
		path string,
		value string,
	) error
}
