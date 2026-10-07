// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"io"
	"runtime"
	"strings"
	"testing"
)

// A line at a time, as a terminal hands one over, or as a writer that writes
// a line and waits does.
type aLineAtATime struct{ rest string }

func (l *aLineAtATime) Read(p []byte) (int, error) {
	if l.rest == "" {
		return 0, io.EOF
	}
	n := strings.IndexByte(l.rest, '\n') + 1
	if n == 0 {
		n = len(l.rest)
	}
	n = copy(p, l.rest[:n])
	l.rest = l.rest[n:]
	return n, nil
}

// One dialect reads input that is not a terminal a block at a time and throws
// away what is left of the block when a line is refused or given up. Measured
// 2026-10-07 with dash 0.5.12, whose block is 1024 bytes on macOS and 8192 on
// Linux; bash, zsh, ksh93 and BusyBox ash lose nothing (#6322). See
// interp.Semantics.PromptErrorDiscardsTheRestOfTheReadBlock.
func TestAnErrorThrowsAwayTheRestOfTheReadBlock(t *testing.T) {
	block := readBlockSize(runtime.GOOS)
	// `echo Z` starting at byte at, after a refused first line and blank
	// lines up to it.
	at := func(at int) string {
		return "fi\n" + strings.Repeat("\n", at-3) + "echo Z\n"
	}
	for _, c := range []struct {
		name    string
		discard bool
		in      io.Reader
		want    string
	}{
		{"a refused line takes the rest of its block", true, strings.NewReader("fi\necho A\necho B\n"), ""},
		{"so does a line given up as it ran", true, strings.NewReader("echo ${nope?gone}\necho A\n"), ""},
		{"a line that only fails takes nothing", true, strings.NewReader("false\necho A\n"), "A\n"},
		{"a line that ends a byte short of the block loses the next", true, strings.NewReader(at(block - 1)), ""},
		{"and the next block is read", true, strings.NewReader(at(block)), "Z\n"},
		{"a read that held one line has nothing left to lose", true, &aLineAtATime{"fi\necho A\necho B\n"}, "A\nB\n"},
		{"the dialect that keeps reading loses nothing", false, strings.NewReader("fi\necho A\necho ${nope?gone}\necho B\n"), "A\nB\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var ran, said strings.Builder
			r := newTestRunner(map[string]string{"PS1": "", "PS2": ""})
			r.Stdout = &ran
			r.Stderr = &said
			s := Shell{
				Runner:                             r,
				In:                                 c.in,
				Out:                                &ran,
				Err:                                &said,
				ErrorDiscardsTheRestOfTheReadBlock: c.discard,
			}
			if _, err := s.Run(t.Context()); err != nil {
				t.Fatal(err)
			}
			if ran.String() != c.want {
				t.Errorf("stdout %q, want %q (stderr %q)", ran.String(), c.want, said.String())
			}
		})
	}
}

// And a session with no error in it reads everything, in blocks or not.
func TestAnOrdinarySessionLosesNothingToTheReadBlock(t *testing.T) {
	var text, want strings.Builder
	for range 3000 {
		text.WriteString("echo line\n")
		want.WriteString("line\n")
	}
	var ran strings.Builder
	r := newTestRunner(map[string]string{"PS1": ""})
	r.Stdout = &ran
	s := Shell{
		Runner:                             r,
		In:                                 strings.NewReader(text.String()),
		Out:                                &ran,
		Err:                                io.Discard,
		ErrorDiscardsTheRestOfTheReadBlock: true,
	}
	if _, err := s.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if ran.String() != want.String() {
		t.Errorf("ran %d bytes, want %d", ran.Len(), want.Len())
	}
}

func TestTheReadBlockIsTheCLibrarysBuffer(t *testing.T) {
	if got := readBlockSize("darwin"); got != 1024 {
		t.Errorf("darwin: %d, want 1024", got)
	}
	if got := readBlockSize("linux"); got != 8192 {
		t.Errorf("linux: %d, want 8192", got)
	}
}
