package console

import (
	"encoding/json"
	"io"
	"sync"

	"github.com/gin-gonic/gin"
)

type frames struct {
	mu     sync.Mutex
	writer gin.ResponseWriter
	seen   int
}

func (f *frames) stream(
	name string,
) io.Writer {
	return &frameStream{frames: f, name: name}
}

func (f *frames) accept(
	stream string,
	data []byte,
) {
	f.mu.Lock()
	defer f.mu.Unlock()

	before := f.seen
	f.seen += len(data)
	if before >= outputLimit {
		return
	}
	keep := min(len(data), outputLimit-before)
	f.write(map[string]any{"type": "out", "stream": stream, "data": string(data[:keep])})
	if keep < len(data) {
		f.write(map[string]any{"type": "out", "stream": "stderr", "data": truncationNote})
	}
}

func (f *frames) exit(
	code int,
	message string,
) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.write(map[string]any{"type": "exit", "code": code, "error": message})
}

func (f *frames) write(
	frame map[string]any,
) {
	line, _ := json.Marshal(frame)
	_, _ = f.writer.Write(append(line, '\n'))
	f.writer.Flush()
}
