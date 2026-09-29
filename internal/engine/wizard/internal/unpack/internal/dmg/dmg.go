package dmg

import "io"

const trailerSize = 512

func Is(
	src io.ReaderAt,
	size int64,
) bool {
	trailer := make([]byte, 4)
	_, err := src.ReadAt(trailer, size-trailerSize)

	return err == nil && string(trailer) == "koly"
}
