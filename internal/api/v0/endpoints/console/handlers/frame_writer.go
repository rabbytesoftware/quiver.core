package console

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/rabbytesoftware/quiver.core/internal/console/command"
)

type frameWriter struct {
	mu        sync.Mutex
	out       http.ResponseWriter
	flusher   http.Flusher
	limit     int
	written   int
	truncated bool
	closed    bool
	broken    bool
}

func newFrameWriter(
	out http.ResponseWriter,
	limit int,
) *frameWriter {
	flusher, _ := out.(http.Flusher)
	return &frameWriter{out: out, flusher: flusher, limit: limit}
}

func (w *frameWriter) stream(
	name string,
) *streamWriter {
	return &streamWriter{frames: w, name: name}
}

func (w *frameWriter) output(
	stream string,
	data []byte,
) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed || w.broken || w.truncated {
		return
	}
	if w.written+len(data) > w.limit {
		w.truncate()
		return
	}

	w.written += len(data)
	w.emit(execOutFrame{Type: "out", Stream: stream, Data: string(data)})
}

func (w *frameWriter) truncate() {
	w.truncated = true
	notice := fmt.Sprintf("\n[output truncated: limit %d KiB]\n", w.limit/1024)
	w.emit(execOutFrame{Type: "out", Stream: "stderr", Data: notice})
}

func (w *frameWriter) exit(
	result command.Result,
) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return
	}
	w.closed = true
	w.emit(execExitFrame{Type: "exit", Code: result.Code, Error: result.Err})
}

func (w *frameWriter) emit(
	frame any,
) {
	if w.broken {
		return
	}

	line, err := json.Marshal(frame)
	if err != nil {
		w.broken = true
		return
	}

	if _, err := w.out.Write(append(line, '\n')); err != nil {
		w.broken = true
		return
	}
	if w.flusher != nil {
		w.flusher.Flush()
	}
}
