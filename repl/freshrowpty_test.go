// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"os"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/pty"
	"github.com/blairham/sh/interp"
)

// The mark for output that never ended its line, through a real terminal.
//
// The reader-driven tests beside this one say what the editor writes when it
// is told a width. They cannot say that a width arrives at all: the padding is
// the screen's width less the mark's, it comes from an ioctl on the terminal
// the session is reading, and from a test that hands the editor a `func() int`
// a width that never arrives looks exactly like one that works. This is the
// only place the ioctl is in the loop — and a width of zero writes no mark,
// so that failure is silent by construction.
//
// It waits on the prompt's command number the way every session test here
// does, which is a mark that has not been on the screen before, and never
// clears the buffer between waits.

// markingSession is a session told to mark unfinished output, on an
// eighty-column terminal.
//
// The option names are this test's own. repl names no shell: what it holds is
// that *some* option controls each half, and which names those are is
// dialect/zsh's to say. The namespace answers `known` for exactly these two,
// so a lookup that folded, trimmed or guessed would show up here as a mark
// that never arrives.
func markingSession(t *testing.T, cols int, on map[string]bool) *session {
	t.Helper()
	return newSessionWith(t, func(s *Shell) {
		f, ok := s.In.(*os.File)
		if !ok {
			t.Fatal("the session's input is not a terminal file")
		}
		if err := pty.SetSize(f, 24, cols); err != nil {
			t.Skipf("no terminal size: %v", err)
		}
		s.Editor.MarkUnfinishedOutputOption = "markpartialline"
		s.Editor.ReturnBeforeThePromptOption = "returnbeforeprompt"
		s.Editor.UnfinishedOutputMark = ptyMark
		s.Editor.ClearBeforeThePrompt = ptyClear
		s.Runner.SetOptionNamespace(func(_ *interp.Runner, name string) (bool, bool) {
			state, known := on[name]
			return state, known
		})
	})
}

// One column wide and four bytes long, so that a padding counted in bytes and
// a padding counted in columns cannot both be right.
const (
	ptyMark = "\x1b[7m%\x1b[0m"
	// What this test's dialect writes on the ground the prompt goes on.
	ptyClear = "\x1b[0m\x1b[27m\x1b[24m\x1b[J"
)

func bothOn() map[string]bool {
	return map[string]bool{"markpartialline": true, "returnbeforeprompt": true}
}

func TestAPromptIsMarkedOffFromUnfinishedOutputThroughATerminal(t *testing.T) {
	const cols = 80
	se := markingSession(t, cols, bothOn())
	// One command, so that a second prompt is drawn after output ran.
	se.typeLine("printf X\n")
	se.end()
	screen := se.screen.String()

	// The whole sequence, with the padding taken from the terminal rather than
	// from anything this test told the editor. The two halves are adjacent
	// here because this session has no prompt hook printing between them.
	want := ptyMark + strings.Repeat(" ", cols-1) + "\r \r" + "\r" + ptyClear
	if !strings.Contains(screen, want) {
		t.Fatalf("the sequence never reached the terminal.\n got %q\nwant %q", screen, want)
	}
	// And it is drawn before every prompt rather than once at startup — the
	// editor cannot ask a terminal where the cursor is, so the sequence is
	// unconditional and erases itself when there was nothing to mark.
	if n := strings.Count(screen, want); n < 2 {
		t.Errorf("the sequence was written %d times, want one before each prompt", n)
	}
}

// A width that is not eighty, because a constant that happens to match the
// terminal's default would pass whatever the editor read.
func TestTheMarksPaddingIsTheTerminalsOwnWidth(t *testing.T) {
	const cols = 51
	se := markingSession(t, cols, bothOn())
	se.typeLine("printf X\n")
	se.end()
	screen := se.screen.String()
	if want := ptyMark + strings.Repeat(" ", cols-1) + "\r \r"; !strings.Contains(screen, want) {
		t.Errorf("padded to something other than %d columns: %q", cols, screen)
	}
	if wrong := ptyMark + strings.Repeat(" ", 79) + "\r"; strings.Contains(screen, wrong) {
		t.Error("padded to 80 columns on a 51-column terminal — the width is not the terminal's")
	}
	// Columns and not bytes: the mark is four bytes and one column, so a
	// padding of `cols` less its length would be three columns longer.
	if wrong := ptyMark + strings.Repeat(" ", cols-len(ptyMark)) + "\r"; strings.Contains(screen, wrong) {
		t.Error("the padding was counted in bytes, not columns")
	}
}

// A continuation prompt is not marked, through the same terminal.
//
// Three prompts are drawn for two commands' worth of typing — the first
// command's, the continuation in the middle of the second, and the second
// command's — and the mark belongs to two of them.
func TestAContinuationPromptIsNotMarkedThroughATerminal(t *testing.T) {
	se := markingSession(t, 80, bothOn())
	// A construct left open by the first newline, finished by the second.
	// Typed as one string so the continuation prompt is reached without
	// waiting for a command number that cannot arrive until the loop closes.
	se.typeLine("for i in 1\ndo printf X; done\n")
	se.end()
	screen := se.screen.String()
	if got := strings.Count(screen, ptyMark); got != 2 {
		t.Errorf("the mark was drawn %d times, want 2 — the continuation prompt takes none.\n%q",
			got, screen)
	}
	// The continuation prompt was really reached, or the count above is 2 for
	// the wrong reason.
	if !strings.Contains(screen, "> ") {
		t.Errorf("no continuation prompt was drawn: %q", screen)
	}
}

// Each option on its own, through the terminal, because the two are not
// independent and the dependence is the part a dialect gets wrong.
func TestTheTwoOptionsThroughATerminal(t *testing.T) {
	for _, c := range []struct {
		name string
		on   map[string]bool
		mark bool
		ret  bool
	}{
		{"both on", bothOn(), true, true},
		{
			"marking off keeps the return",
			map[string]bool{"markpartialline": false, "returnbeforeprompt": true},
			false, true,
		},
		{
			// The return is the outer of the two: with it off the mark is not
			// written either, though the marking option is still on.
			"the return off takes the mark with it",
			map[string]bool{"markpartialline": true, "returnbeforeprompt": false},
			false, false,
		},
		{
			"both off",
			map[string]bool{"markpartialline": false, "returnbeforeprompt": false},
			false, false,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			se := markingSession(t, 80, c.on)
			se.typeLine("printf X\n")
			se.end()
			screen := se.screen.String()
			if got := strings.Contains(screen, ptyMark); got != c.mark {
				t.Errorf("marked = %v, want %v: %q", got, c.mark, screen)
			}
			// The erase is neither option's doing and is written in every
			// row, which is what says it is a third answer.
			if !strings.Contains(screen, ptyClear) {
				t.Errorf("the rows below the prompt were not erased: %q", screen)
			}
			if c.ret && !c.mark {
				// The return alone, with no padding in front of it.
				if strings.Contains(screen, strings.Repeat(" ", 40)) {
					t.Errorf("padding was written with the marking off: %q", screen)
				}
			}
		})
	}
}

// The session marks the previous command's output *before* it runs the prompt
// hooks, through a real terminal and the real loop.
//
// This is the composition the unit tests cannot reach. They can say that the
// editor's two halves write the right bytes in the right order when something
// calls them in that order; only the loop can say that it calls them in that
// order, and the loop is where the ordering decision lives.
//
// Measured 2026-09-12 against the shell that marks, with a `precmd` printing
// without a newline and a command doing the same:
//
//	CMD  mark <79 spaces> \r <space> \r  HOOK  \r  <erase>  P>
//
// The hook's output falls between the mark and the return. A session that
// marked after its hooks would mark the hook's own half-written line and leave
// the command's output with the prompt drawn against it — the reported bug
// with an extra step, since the progress line a plugin manager prints while it
// loads is exactly such a hook (#2477).
func TestTheSessionMarksBeforeItRunsThePromptHooks(t *testing.T) {
	se := newSessionWith(t, func(s *Shell) {
		f, ok := s.In.(*os.File)
		if !ok {
			t.Fatal("the session's input is not a terminal file")
		}
		if err := pty.SetSize(f, 24, 80); err != nil {
			t.Skipf("no terminal size: %v", err)
		}
		s.Editor.MarkUnfinishedOutputOption = "markpartialline"
		s.Editor.ReturnBeforeThePromptOption = "returnbeforeprompt"
		s.Editor.UnfinishedOutputMark = ptyMark
		s.Editor.ClearBeforeThePrompt = ptyClear
		s.Runner.SetOptionNamespace(func(_ *interp.Runner, name string) (bool, bool) {
			return true, name == "markpartialline" || name == "returnbeforeprompt"
		})
		// The hook writes where the editor writes, so that the order the two
		// reach the terminal is what this reads back. Everywhere else in these
		// tests they are separate buffers, which is what keeps a command's
		// output out of assertions about the screen.
		s.Runner.Stdout = s.Out
		s.Hooks = HookStyle{BeforePrompt: "precmd"}
		if !s.Runner.DefineFunctionFromText("precmd", `printf HOOK`) {
			t.Fatal("defining precmd")
		}
	})
	se.typeLine("printf CMD\n")
	se.end()
	screen := se.screen.String()

	// The last prompt drawn is the one to read: the hook has run at each of
	// them, and the mark belongs to the command's output before this one.
	last := strings.LastIndex(screen, ptyMark)
	if last < 0 {
		t.Fatalf("nothing was ever marked: %q", screen)
	}
	tail := screen[last:]
	hook := strings.Index(tail, "HOOK")
	ground := strings.Index(tail, "\r"+ptyClear)
	if hook < 0 || ground < 0 {
		t.Fatalf("the hook or the ground never arrived after a mark: %q", tail)
	}
	if hook > ground {
		t.Errorf("the hook printed after the return and the erase, want between them: %q", tail)
	}
	// And the whole shape, so that a rearrangement keeping every byte fails.
	want := ptyMark + strings.Repeat(" ", 79) + "\r \r" + "HOOK" + "\r" + ptyClear
	if !strings.Contains(screen, want) {
		t.Errorf("the session wrote\n %q\nwant it to contain\n %q", screen, want)
	}
}
