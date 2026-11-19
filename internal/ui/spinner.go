package ui

import (
	"io"
	"sync"
	"time"
)

var frames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Spinner shows that a request is in flight. It draws on one line of a
// terminal stream (stderr) and erases itself when stopped, so it never ends
// up in captured output. It only appears after a short delay, so fast
// responses do not flicker.
type Spinner struct {
	w     io.Writer
	p     Painter
	delay time.Duration
	// MaxWidth is the terminal width; longer labels are truncated so the
	// line never wraps (a wrapped line cannot be erased).
	MaxWidth int

	mu      sync.Mutex
	label   string
	drawn   bool
	stop    chan struct{}
	stopped chan struct{}
}

// NewSpinner returns a spinner that writes to w (a terminal).
func NewSpinner(w io.Writer, p Painter) *Spinner {
	return &Spinner{w: w, p: p, delay: 150 * time.Millisecond}
}

// Start begins animating with label. Calling Start on a running spinner
// only changes the label.
func (s *Spinner) Start(label string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.label = label
	if s.stop != nil {
		return
	}
	s.stop, s.stopped = make(chan struct{}), make(chan struct{})
	go s.run(s.stop, s.stopped)
}

// Update changes the label of a running spinner.
func (s *Spinner) Update(label string) {
	s.mu.Lock()
	s.label = label
	s.mu.Unlock()
}

// Stop erases the spinner. It is safe to call more than once.
func (s *Spinner) Stop() {
	s.mu.Lock()
	stop, stopped := s.stop, s.stopped
	s.stop = nil
	s.mu.Unlock()
	if stop == nil {
		return
	}
	close(stop)
	<-stopped
}

func (s *Spinner) run(stop, stopped chan struct{}) {
	defer close(stopped)
	timer := time.NewTimer(s.delay)
	defer timer.Stop()
	select {
	case <-stop:
		return
	case <-timer.C:
	}
	tick := time.NewTicker(80 * time.Millisecond)
	defer tick.Stop()
	io.WriteString(s.w, "\x1b[?25l") // hide the cursor while spinning
	defer func() {
		io.WriteString(s.w, "\r\x1b[2K\x1b[?25h")
	}()
	for i := 0; ; i++ {
		s.mu.Lock()
		label := s.label
		s.mu.Unlock()
		if s.MaxWidth > 6 {
			label = Truncate(label, s.MaxWidth-4)
		}
		io.WriteString(s.w, "\r\x1b[2K "+s.p.Fg(Accent, frames[i%len(frames)])+" "+s.p.Muted(label))
		select {
		case <-stop:
			return
		case <-tick.C:
		}
	}
}
