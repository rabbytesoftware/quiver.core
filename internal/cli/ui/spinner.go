package ui

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// SpinnerFrames are the braille frames shared by the spinner and the lifecycle
// progress view.
var SpinnerFrames = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

// Spinner renders an indeterminate single-line spinner to w, but only after
// delay elapses, so operations that finish quickly render nothing. It writes a
// carriage return before each frame and clears the line on Stop.
type Spinner struct {
	w     io.Writer
	label string
	delay time.Duration

	stop chan struct{}
	done chan struct{}
	once sync.Once

	mu      sync.Mutex
	printed bool
}

// NewSpinner builds a spinner that writes to w and starts drawing after delay.
func NewSpinner(w io.Writer, label string, delay time.Duration) *Spinner {
	return &Spinner{
		w:     w,
		label: label,
		delay: delay,
		stop:  make(chan struct{}),
		done:  make(chan struct{}),
	}
}

// Start launches the render loop. It returns immediately; nothing is written
// until delay passes.
func (s *Spinner) Start() { go s.run() }

func (s *Spinner) run() {
	defer close(s.done)

	select {
	case <-time.After(s.delay):
	case <-s.stop:
		return // stopped before the delay: never drew anything
	}

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	frame := 0
	s.draw(frame)
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			frame++
			s.draw(frame)
		}
	}
}

func (s *Spinner) draw(frame int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.printed = true
	_, _ = fmt.Fprintf(s.w, "\r%s %s",
		Brand.Render(SpinnerFrames[frame%len(SpinnerFrames)]),
		Muted.Render(s.label))
}

// Stop halts the spinner and clears its line if anything was drawn. It is safe
// to call more than once, and must be called after Start.
func (s *Spinner) Stop() {
	s.once.Do(func() {
		close(s.stop)
		<-s.done
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.printed {
			_, _ = fmt.Fprint(s.w, "\r\033[K") // carriage return + clear to end of line
		}
	})
}
