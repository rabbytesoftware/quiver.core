package shelf

type userPath interface {
	read() (string, error)
	write(
		value string,
	) error
}
