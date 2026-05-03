package storage

// Unmarshal exposes the unexported unmarshal function for white-box tests.
func Unmarshal(data []byte) (*ViewModel, error) {
	return unmarshal(data)
}
