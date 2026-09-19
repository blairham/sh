// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// `vared` at a prompt: the half #2914 named and could not reach.
//
// vared_test.go is the whole of what a script sees, which is the refusals.
// This is the other half — a runner with a terminal and an editor behind it —
// and what each row asserts was measured 2026-09-18 through a pseudo-terminal
// against zsh 5.9.2. The table is in vared.go beside the builtin.
//
// The editor here is a stub rather than repl's, and that is the right seam to
// test at: what this package owes is the *request* it makes and what it does
// with the answer. That the terminal really goes back into raw mode for the
// read is repl's, and repl's own pty test is where it is asserted — a stub
// there would be the fake that hides a broken feature.

// varedEdit runs src with a terminal and an editor that answers one request,
// and hands back what was printed, the status, and the request the builtin
// made.
func varedEdit(t *testing.T, src, answer string, end interp.LineEditEnd) (out string, status int, asked interp.LineEdit) {
	t.Helper()
	f := preset.Parse(t, src)
	var buf strings.Builder
	r := preset.Runner(dialecttest.Base{
		Stdout: &buf, Stderr: &buf, Dir: t.TempDir(), Terminal: true,
	})
	r.EditLine = func(req interp.LineEdit) (string, interp.LineEditEnd) {
		asked = req
		return answer, end
	}
	st, err := r.Run(context.Background(), f)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return buf.String(), st, asked
}

// What the line starts from, one shape of value at a time.
//
// The join is on the **first character of IFS** and not on a space, which is
// the row a guess gets wrong: `IFS=:` with `a=(one two)` draws `one:two` in
// zsh, the same rule `$*` joins on.
func TestVaredPutsTheValueInTheLine(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"a scalar", `v=hello; vared v`, "hello"},
		{"an array, joined", `a=(one two); vared a`, "one two"},
		{"an array under another IFS", `IFS=:; a=(one two); vared a`, "one:two"},
		{"one element of an array", `a=(one two); vared 'a[2]'`, "two"},
		{"an association's pairs", `typeset -A m=(k1 v1); vared m`, "k1 v1"},
		{"a name being created", `vared -c newv`, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, _, asked := varedEdit(t, c.src, "", interp.LineEditAccepted)
			if asked.Initial != c.want {
				t.Errorf("the line started from %q, want %q", asked.Initial, c.want)
			}
		})
	}
}

// And what is stored is decided by the kind that is already there, with the
// `-a` and `-A` letters deciding only for a name that is not.
func TestVaredStoresWhatWasAccepted(t *testing.T) {
	for _, c := range []struct{ name, src, edited, accepted, probe, want string }{
		{"a scalar", `v=hello`, "v", "helloXY", `print -r -- "[$v]"`, "[helloXY]"},
		{
			"an array is split again", `a=(one two)`, "a", "one two three",
			`print -r -- "${#a} [$a[3]]"`, "3 [three]",
		},
		{
			"and split on IFS", `IFS=:; a=(x)`, "a", "p:q r",
			`print -r -- "${#a} [$a[2]]"`, "2 [q r]",
		},
		{
			"one element, and only that element", `a=(one two three)`, `'a[2]'`, "twoX",
			`print -r -- "${#a} [$a[2]] [$a[3]]"`, "3 [twoX] [three]",
		},
		{
			"an association's pairs", `typeset -A m=(k1 v1)`, "m", "k2 v2",
			`print -r -- "[$m[k2]] [$m[k1]]"`, "[v2] []",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st, _ := varedEdit(t,
				c.src+"; vared "+c.edited+"; "+c.probe, c.accepted, interp.LineEditAccepted)
			if st != 0 {
				t.Errorf("status = %d, want 0", st)
			}
			if got := strings.TrimSuffix(out, "\n"); got != c.want {
				t.Errorf("after accepting %q the shell says %q, want %q", c.accepted, got, c.want)
			}
		})
	}
}

// `-c` creates the name, and `-a` and `-A` say which kind — which is the whole
// of what those two letters are for and why they are ignored without it.
func TestVaredCreatesTheKindTheLettersName(t *testing.T) {
	for _, c := range []struct{ name, src, accepted, want string }{
		{"a scalar", `vared -c newv; print -r -- "[$newv]"`, "made", "[made]"},
		{
			"an array", `vared -c -a newa; print -r -- "${#newa} [$newa[2]]"`,
			"p q r", "3 [q]",
		},
		{
			"an association", `vared -c -A newm; print -r -- "[$newm[kk]]"`,
			"kk vv", "[vv]",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st, _ := varedEdit(t, c.src, c.accepted, interp.LineEditAccepted)
			if st != 0 {
				t.Errorf("status = %d, want 0", st)
			}
			if got := strings.TrimSuffix(out, "\n"); got != c.want {
				t.Errorf("output = %q, want %q", got, c.want)
			}
		})
	}
}

// The three letters that change the read rather than the value.
func TestVaredSaysWhatTheReadIs(t *testing.T) {
	for _, c := range []struct {
		name, src string
		want      interp.LineEdit
	}{
		{"a plain read", `v=x; vared v`, interp.LineEdit{Initial: "x"}},
		{"a prompt", `v=x; vared -p 'P ' v`, interp.LineEdit{Prompt: "P ", Initial: "x"}},
		// The argument attached to the letter rather than standing after it,
		// which is the spelling a cluster forces.
		{"an attached prompt", `v=x; vared -pP: v`, interp.LineEdit{Prompt: "P:", Initial: "x"}},
		{"history", `v=x; vared -h v`, interp.LineEdit{Initial: "x", History: true}},
		{
			"end of input ends it", `v=x; vared -e v`,
			interp.LineEdit{Initial: "x", EndOnEndOfInput: true},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, _, asked := varedEdit(t, c.src, "", interp.LineEditAccepted)
			if asked != c.want {
				t.Errorf("the read asked for %+v, want %+v", asked, c.want)
			}
		})
	}
}

// An interrupt is 130 and abandons what the shell was reading.
//
// **#2914 records this as status 1**, and the measurement says otherwise:
// `vared v; print after` interrupted with `^C` in zsh 5.9.2 prints nothing and
// leaves 130 behind. The `print` is the discriminating half — a status alone
// would not tell an interrupt that failed one command from one that abandoned
// the line.
func TestVaredInterruptedAbandonsTheLine(t *testing.T) {
	out, st, _ := varedEdit(t,
		`v=hello; vared v; print after`, "", interp.LineEditInterrupted)
	if st != 130 {
		t.Errorf("status = %d, want 130", st)
	}
	if out != "" {
		t.Errorf("output = %q, want nothing — the rest of the line must not run", out)
	}
}

// End of input is 1, leaves the variable alone, and does *not* abandon the
// line — which is the difference from the interrupt above.
func TestVaredEndedByEndOfInputLeavesTheValue(t *testing.T) {
	out, st, _ := varedEdit(t,
		`v=hello; vared -e v; print -r -- "st=$? [$v]"`, "", interp.LineEditEndOfInput)
	if st != 0 {
		t.Errorf("the line's status = %d, want 0 — the `print` runs", st)
	}
	if want := "st=1 [hello]\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// The letters this editor has no half of are refused by name rather than
// ignored, and only in a session: a script still reaches the terminal refusal
// first, which is measured and is the whole of what a script sees.
func TestVaredRefusesTheLettersItCannotHonor(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"a terminal of its own", `v=x; vared -t /dev/tty v`, "zsh:vared:1: -t is not implemented yet\n"},
		{"a right-hand prompt", `v=x; vared -r 'R' v`, "zsh:vared:1: -r is not implemented yet\n"},
		{"a keymap", `v=x; vared -M viins v`, "zsh:vared:1: -M is not implemented yet\n"},
		{"a widget at each end", `v=x; vared -i w v`, "zsh:vared:1: -i is not implemented yet\n"},
		// The first letter named is what the refusal says, which is the order
		// they were read in.
		{"the first of two", `v=x; vared -r R -t /dev/tty v`, "zsh:vared:1: -r is not implemented yet\n"},
		// The control: a letter this does honor is not refused, so the check
		// is about the letters rather than about there being any.
		{"and not one it honors", `v=x; vared -h v; print ran`, "ran\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _, _ := varedEdit(t, c.src, "x", interp.LineEditAccepted)
			if out != c.want {
				t.Errorf("output = %q, want %q", out, c.want)
			}
		})
	}
}

// And with no editor behind the terminal the sentence says which half is
// missing, rather than claiming the terminal is unreachable.
func TestVaredWithATerminalAndNoEditorSaysSo(t *testing.T) {
	f := preset.Parse(t, `v=x; vared v`)
	var buf strings.Builder
	r := preset.Runner(dialecttest.Base{
		Stdout: &buf, Stderr: &buf, Dir: t.TempDir(), Terminal: true,
	})
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	want := "zsh:vared:1: the line editor cannot be re-entered from a command yet\n"
	if buf.String() != want {
		t.Errorf("output = %q, want %q", buf.String(), want)
	}
}
