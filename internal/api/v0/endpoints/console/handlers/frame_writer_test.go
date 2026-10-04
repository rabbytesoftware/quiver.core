package console

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/console/command"
)

type failingResponse struct {
	header http.Header
	writes int
}

func (f *failingResponse) Header() http.Header { return f.header }

func (f *failingResponse) WriteHeader(int) {}

func (f *failingResponse) Write([]byte) (int, error) {
	f.writes++
	return 0, errors.New("client went away")
}

type plainResponse struct {
	header http.Header
	body   strings.Builder
}

func (p *plainResponse) Header() http.Header { return p.header }

func (p *plainResponse) WriteHeader(int) {}

func (p *plainResponse) Write(b []byte) (int, error) { return p.body.Write(b) }

func TestFrameWriter_Output_StopsAfterATransportWriteError(t *testing.T) {
	out := &failingResponse{header: http.Header{}}
	w := newFrameWriter(out, 1024)

	w.output("stdout", []byte("one"))
	w.output("stdout", []byte("two"))
	w.exit(command.Result{})

	assert.Equal(t, 1, out.writes, "nothing may be written after the first failure")
}

func TestFrameWriter_Exit_OnlyTheFirstExitIsEmitted(t *testing.T) {
	rec := httptest.NewRecorder()
	w := newFrameWriter(rec, 1024)

	w.exit(command.Result{Code: 2, Err: "first"})
	w.exit(command.Result{Code: 3, Err: "second"})

	assert.Equal(t, 1, strings.Count(rec.Body.String(), `"type":"exit"`))
	assert.Contains(t, rec.Body.String(), `"first"`)
}

func TestFrameWriter_Emit_FlushesAfterEveryFrame(t *testing.T) {
	rec := httptest.NewRecorder()
	w := newFrameWriter(rec, 1024)

	w.output("stdout", []byte("a"))
	assert.True(t, rec.Flushed)
}

func TestFrameWriter_Emit_WorksWithoutAFlusher(t *testing.T) {
	out := &plainResponse{header: http.Header{}}
	w := newFrameWriter(out, 1024)

	w.output("stderr", []byte("x"))
	w.exit(command.Result{})

	assert.Equal(t, 2, strings.Count(out.body.String(), "\n"))
}

func TestFrameWriter_Emit_AFrameThatCannotBeEncodedBreaksTheWriter(t *testing.T) {
	out := &plainResponse{header: http.Header{}}
	w := newFrameWriter(out, 1024)

	w.emit(make(chan int))
	w.output("stdout", []byte("after"))

	assert.Empty(t, out.body.String())
}

func TestFrameWriter_Output_TheTruncationNoticeIsSentOnce(t *testing.T) {
	rec := httptest.NewRecorder()
	w := newFrameWriter(rec, 4)

	w.output("stdout", []byte("12345"))
	w.output("stdout", []byte("6789"))

	assert.Equal(t, 1, strings.Count(rec.Body.String(), "output truncated"))
}
