// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/internal/pty"
	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `-t` asked of a descriptor that really is a terminal (#1967).
//
// It was a fixed `false`, in `test`, `[` and `[[ ]]` alike, so every `[[ -t 1
// ]]` in a startup file took the non-terminal arm in a real session — the
// branch that decides whether to draw color, load a prompt theme or install
// completions — with no diagnostic to say so.
//
// The terminal is a pseudo-terminal this test opens rather than whatever the
// run was handed, which is what makes the answer an assertion instead of a
// property of the machine: `go test` in CI has no terminal at all, and one
// inherited from a developer's shell is not a thing to write `true` against.
//
// Measured 2026-09-10 on a pseudo-terminal, `[ -t 0 ]`, `test -t 0`, `[ -t 1
// ]`, `[ -t 2 ]` and `[[ -t 0 ]]`: true in dash, bash 5.3.15, bash-as-sh, bash
// 3.2.57, ksh93 and zsh 5.9.2 — every column of the panel, and dash only for
// the three that are not `[[ ]]`, which it does not have.
func TestTerminalDescriptorIsATerminal(t *testing.T) {
	control, terminal, err := pty.Open()
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	defer func() { _ = control.Close() }()
	defer func() { _ = terminal.Close() }()

	// One run answering about three descriptors, two of which are terminals
	// and one of which is not. A shell that had gone on answering a fixed
	// false would fail the first two; one that answered a fixed true would
	// fail the third, and a test that asserted only the terminal could not
	// tell those apart.
	//
	// Standard output is the buffer the assertion reads, so descriptor 1 is
	// the not-a-terminal, and the terminal sits on 0 and 2. Nothing is
	// written to or read from the pseudo-terminal: `-t` is an ioctl and the
	// question has an answer with no bytes moving in either direction, which
	// is what keeps this test free of the waiting a pty test usually needs.
	const src = `[ -t 0 ] && echo B0T || echo B0F
test -t 2 && echo T2T || echo T2F
[ -t 1 ] && echo B1T || echo B1F
[[ -t 0 ]] && echo BB0T || echo BB0F
[[ -t 1 ]] && echo BB1T || echo BB1F`
	out, status := runGrammar(t, src, func(d *syntax.Dialect) { d.DoubleBracket = true }, func(r *Runner) {
		r.Stdin, r.Stderr = terminal, terminal
	})
	const want = "B0T\nT2T\nB1F\nBB0T\nBB1F\n"
	if out != want || status != 0 {
		t.Errorf("got %q at %d, want %q at 0", out, status, want)
	}
}

// And the descriptors past the named three, which are the shell's own table
// rather than the process's.
//
// `exec 3<&0` duplicates whatever standard input holds, so 3 is the terminal
// without this test reaching for a path or a number; `exec 4< file` puts a
// regular file at 4; and 9 has nothing open at it at all. Measured 2026-09-10
// on a pseudo-terminal, all six panel columns: the duplicate is true, the file
// is false, and the number nothing is open at is false.
func TestTerminalTestReachesTheShellsOwnDescriptors(t *testing.T) {
	control, terminal, err := pty.Open()
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	defer func() { _ = control.Close() }()
	defer func() { _ = terminal.Close() }()

	dir := t.TempDir()
	path := filepath.Join(dir, "f")
	if err := os.WriteFile(path, []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	src := `exec 3<&0
exec 4< ` + path + `
[ -t 3 ] && echo DUP-T || echo DUP-F
[ -t 4 ] && echo FILE-T || echo FILE-F
[ -t 9 ] && echo NONE-T || echo NONE-F
[ -t -1 ] && echo NEG-T || echo NEG-F`
	out, status := run(t, src, func(r *Runner) {
		r.Stdin, r.Stderr = terminal, terminal
	})
	const want = "DUP-T\nFILE-F\nNONE-F\nNEG-F\n"
	if out != want || status != 0 {
		t.Errorf("got %q at %d, want %q at 0", out, status, want)
	}
}

// A lone `-t` is `-t 1` in two of the panel and a non-empty string in four
// (Semantics.BareTerminalTestIsDescriptorOne).
//
// Measured 2026-09-10 with `[ -t ] >/dev/null`, which pins the descriptor to
// something that is certainly not a terminal rather than to whatever the run
// was handed: status 0 in dash, bash 5.3.15, bash-as-sh and bash 3.2.57, and 1
// in ksh93 and zsh 5.9.2. The same line at a pseudo-terminal with nothing
// redirected is 0 in all six — which is what says the two are answering about
// descriptor 1 and not refusing the word.
//
// `-t` alone among the operators: `[ -f ]`, `[ -n ]`, `[ -z ]` and `[ -e ]`
// are 0 in all six columns, so the axis is written for the one word rather
// than as a general rule about an operator with no operand, and those four are
// asserted here so a change that generalized it would be caught.
func TestBareTerminalTestIsDescriptorOneIsAnAxis(t *testing.T) {
	control, terminal, err := pty.Open()
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	defer func() { _ = control.Close() }()
	defer func() { _ = terminal.Close() }()

	for _, src := range []string{`[ -t ]; echo $?`, `test -t; echo $?`, `[ ! -t ]; echo $((1-$?))`} {
		// No: the one-argument string rule, and `-t` is a non-empty string
		// whatever the descriptor is — so the terminal on standard output
		// changes nothing, which is the half that says the axis is doing the
		// work and not the pty.
		for _, out := range []*os.File{nil, terminal} {
			got, _ := run(t, src, func(r *Runner) {
				s := *r.Semantics
				s.BareTerminalTestIsDescriptorOne = No
				r.Semantics = &s
				if out != nil {
					r.Stdout = out
				}
			})
			if out == nil && got != "0\n" {
				t.Errorf("%s with No = %q, want the string rule's 0", src, got)
			}
		}

		// Yes: descriptor 1, which is the buffer here and so false.
		got, _ := run(t, src, func(r *Runner) {
			s := *r.Semantics
			s.BareTerminalTestIsDescriptorOne = Yes
			r.Semantics = &s
		})
		if got != "1\n" {
			t.Errorf("%s with Yes and a buffer = %q, want 1", src, got)
		}
	}

	// And Yes at a real terminal is true, which is the direction a fixed
	// false would still pass without. Standard output is the terminal, so the
	// status is carried out on standard error instead.
	got, _ := run(t, `[ -t ]; echo $? >&2`, func(r *Runner) {
		s := *r.Semantics
		s.BareTerminalTestIsDescriptorOne = Yes
		r.Semantics = &s
		r.Stdout = terminal
	})
	if got != "0\n" {
		t.Errorf("`[ -t ]` with Yes at a terminal = %q, want 0", got)
	}

	// The other one-argument operators, which the axis must not move.
	for _, src := range []string{`[ -f ]`, `[ -n ]`, `[ -z ]`, `[ -e ]`} {
		for name, a := range map[string]Answer{"No": No, "Yes": Yes} {
			_, st := run(t, src, func(r *Runner) {
				s := *r.Semantics
				s.BareTerminalTestIsDescriptorOne = a
				r.Semantics = &s
			})
			if st != 0 {
				t.Errorf("%s with %s = %d, want the string rule's 0", src, name, st)
			}
		}
	}
}
