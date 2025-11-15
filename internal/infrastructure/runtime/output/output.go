package output

import (
	"bytes"
	"sync"
)

// Handler manages process output and error streams
// It provides both buffered (accumulated) and streaming (real-time) access to output
type Handler struct {
	output  *bytes.Buffer
	error   *bytes.Buffer
	outChan chan string
	errChan chan string
	mu      sync.RWMutex
	closed  bool
	closeMu sync.Mutex
}

func NewHandler() *Handler {
	return NewHandlerWithBuffers(200, 100)
}

func NewHandlerWithBuffers(outChanSize, errChanSize int) *Handler {
	return &Handler{
		output:  &bytes.Buffer{},
		error:   &bytes.Buffer{},
		outChan: make(chan string, outChanSize),
		errChan: make(chan string, errChanSize),
		closed:  false,
	}
}

func (h *Handler) WriteOutput(line string) {
	h.mu.Lock()
	h.output.WriteString(line + "\n")
	h.mu.Unlock()

	h.closeMu.Lock()
	closed := h.closed
	h.closeMu.Unlock()

	if !closed {
		select {
		case h.outChan <- line:
			// Successfully sent
		default:
			// Channel full, drop line to prevent blocking
		}
	}
}

func (h *Handler) WriteError(line string) {
	h.mu.Lock()
	h.error.WriteString(line + "\n")
	h.mu.Unlock()

	h.closeMu.Lock()
	closed := h.closed
	h.closeMu.Unlock()

	if !closed {
		select {
		case h.errChan <- line:
			// Successfully sent
		default:
			// Channel full, drop line to prevent blocking
		}
	}
}

func (h *Handler) GetOutput() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.output.String()
}

func (h *Handler) GetError() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.error.String()
}

func (h *Handler) OutChan() <-chan string {
	return h.outChan
}

func (h *Handler) ErrChan() <-chan string {
	return h.errChan
}

func (h *Handler) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.output.Reset()
	h.error.Reset()
}

func (h *Handler) IsClosed() bool {
	h.closeMu.Lock()
	defer h.closeMu.Unlock()
	return h.closed
}

func (h *Handler) Close() {
	h.closeMu.Lock()
	defer h.closeMu.Unlock()

	if h.closed {
		return
	}

	h.closed = true
	close(h.outChan)
	close(h.errChan)
}
