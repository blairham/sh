// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package smoke

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// Screen collects everything the shell drew, so a wait is on a mark in it
// rather than on a duration.
//
// This is the discipline #635 arrived at, and the reason it is written down
// here rather than rediscovered per test: a shell leaves raw mode to run a
// command and takes the terminal back afterwards, and the kernel drops
// whatever is queued but unread when the line discipline changes. A byte
// written between a command's output appearing and the next prompt being drawn
// is simply gone — so the thing to wait on is the *next prompt*, which the
// shell draws after raw mode is restored and immediately before it reads. The
// two events cannot be reordered, which is what makes it a synchronization
// rather than a guess.
//
// A suite that types too early does not hang: it produces convincing false
// failures, with the shell's own output already on the screen.
//
// It never forgets anything. Text is what a failure prints, and a buffer
// cleared between waits would report a screen nobody saw. What moves instead
// is a cursor — see Seek.
type Screen struct {
	mu   sync.Mutex
	buf  strings.Builder
	seen int
}

// Watch drains a stream into a Screen.
//
// It takes an io.Reader rather than the pseudo-terminal itself so the waiting
// can be tested without one.
func Watch(r io.Reader) *Screen {
	s := &Screen{}
	go func() {
		b := make([]byte, 4096)
		for {
			n, err := r.Read(b)
			if n > 0 {
				s.mu.Lock()
				s.buf.Write(b[:n])
				s.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	return s
}

// Text is everything drawn since the session began, escapes and all.
func (s *Screen) Text() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// Draw appends text as though the shell had drawn it. Only a test has any
// business calling it; the reader goroutine is what fills a live Screen.
func (s *Screen) Draw(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf.WriteString(text)
}

// Seek reports whether the mark has been drawn *since the last one was*, and
// moves the cursor past it if so.
//
// The cursor is the whole point. A session draws the same prompt after every
// line, so a plain search over everything drawn answers "yes" from the first
// prompt forever, and a wait meaning "the shell is prompting again" is
// satisfied before the line it typed has even run.
func (s *Screen) Seek(mark string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := strings.Index(s.buf.String()[s.seen:], mark)
	if i < 0 {
		return false
	}
	s.seen += i + len(mark)
	return true
}

// Await blocks until the mark has been drawn, and reports why it gave up
// rather than hanging if it never is.
func (s *Screen) Await(mark string, budget time.Duration) error {
	_, err := s.AwaitAny(budget, mark)
	return err
}

// AwaitAny blocks until any one of the marks has been drawn, and answers which.
//
// Several marks because the first thing a session has to do is recognize a
// prompt it may not have been able to set: the shell's own default is one
// answer and the one from a startup file is another, and which arrived is
// itself a finding.
func (s *Screen) AwaitAny(budget time.Duration, marks ...string) (string, error) {
	deadline := time.Now().Add(budget)
	for {
		for _, mark := range marks {
			if s.Seek(mark) {
				return mark, nil
			}
		}
		if !time.Now().Before(deadline) {
			return "", fmt.Errorf("waited %v for %s; last drawn: %s",
				budget, quoteAll(marks), LastLines(s.Text(), diagnosticLines))
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// Quiet reports whether the mark stays undrawn for the whole of a wait, which
// is how "the shell stopped saying this" is asserted.
//
// It is the assertion suspend needs: a job that has been stopped produces
// nothing, and there is no positive mark for the absence of output.
func (s *Screen) Quiet(mark string, budget time.Duration) bool {
	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		if s.Seek(mark) {
			return false
		}
		time.Sleep(2 * time.Millisecond)
	}
	return true
}

// diagnosticLines is how much of the screen a failed wait prints. Enough to
// show the last prompt and what followed it, short enough to sit under a
// table.
const diagnosticLines = 3

func quoteAll(marks []string) string {
	q := make([]string, len(marks))
	for i, m := range marks {
		q[i] = fmt.Sprintf("%q", m)
	}
	return strings.Join(q, " or ")
}

// LastLines is the end of the screen, rendered for a person to read.
func LastLines(s string, n int) string {
	lines := render(s)
	elided := ""
	if len(lines) > n {
		lines, elided = lines[len(lines)-n:], "… "
	}
	return elided + strings.Join(lines, " ⏎ ")
}

// Readable renders drawn text for a person reading a failure.
//
// Nothing asserts against this: every assertion in this package runs on the
// bytes the shell actually wrote, and only the diagnostic is cleaned up. A
// suite that graded rendered text would pass for a shell whose every line was
// a control character — and would be reading its own renderer's opinion of the
// screen rather than the screen.
func Readable(s string) string { return strings.Join(render(s), "⏎") }

// render replays the drawn bytes as the lines a person would have seen.
//
// It is a very small terminal: a carriage return puts the cursor back at the
// start of the line and what follows *overwrites* what was there, and the
// erase-to-end-of-line sequence truncates. Without that, the report of a
// failed wait is the editor's redraw of every keystroke in turn — `smok`,
// `smoke`, `smokea` — which is forty times the length and says nothing the
// last one does not.
//
// Control bytes that survive are spelled out rather than dropped, because a
// stray one is exactly the kind of thing worth seeing.
func render(s string) []string {
	var lines []string
	var line []byte
	col := 0
	put := func(text string) {
		for i := range len(text) {
			if col < len(line) {
				line[col] = text[i]
			} else {
				line = append(line, text[i])
			}
			col++
		}
	}
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == 0x1b:
			if i+1 < len(s) && s[i+1] == '[' {
				start := i + 2
				i += 2
				for i < len(s) && (s[i] < '@' || s[i] > '~') {
					i++
				}
				// Erase to the end of the line is the one sequence that
				// changes what is on screen rather than where the cursor is,
				// and the editor's redraw is built out of it.
				if i < len(s) && s[i] == 'K' && (i == start || s[start:i] == "0") {
					line = line[:col]
				}
			} else {
				i++ // a two-byte sequence
			}
		case c == '\r':
			col = 0
		case c == '\n':
			lines = append(lines, string(line))
			line, col = nil, 0
		case c == '\t':
			put("\t")
		case c < 0x20 || c == 0x7f:
			put(fmt.Sprintf("\\x%02x", c))
		default:
			put(string(c))
		}
	}
	if len(line) > 0 {
		lines = append(lines, string(line))
	}
	return lines
}
