// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// `-O shopt_option` is the invocation letter that reaches a dialect's
// *second* option namespace, and until #3264 this front end did not have it.
//
// The letter fell through to the runner as an ordinary `set` letter, which
// meant the **next word** — the option's name — had already been taken as the
// script operand. So `bash -O checkhash -c 'shopt checkhash'` did not refuse
// an option it did not have: it went looking for a file called `checkhash`,
// did not find one, and exited 127. A script handing a shopt name to a shell
// built on this front end got "no such file or directory" about a word that
// was never a path.
//
// Everything here drives the front end alone. What the names mean is the
// dialect's — dialect/bash wires them to the `shopt` table — and the two
// halves are tested apart on purpose: this file must fail for a front end
// that stops reading the letter whatever any dialect does with it.

// shellOptionRecorder installs a namespace that writes down what it was asked
// for, so a test can tell "the letter was read" from "the letter was ignored
// and the word became a script".
type shellOptionRecorder struct {
	moved  []string
	listed []bool
	// refuse is a name the mover will not take, answering 2 the way bash
	// answers a name it does not have.
	refuse string
}

func (rec *shellOptionRecorder) install(r *interp.Runner) {
	r.SetShellOptionNamespace(
		func(_ *interp.Runner, name string, on bool) int {
			sign := "-"
			if !on {
				sign = "+"
			}
			rec.moved = append(rec.moved, sign+name)
			if name == rec.refuse {
				return 2
			}
			return 0
		},
		func(rr *interp.Runner, reissuable bool) {
			rec.listed = append(rec.listed, reissuable)
			_, _ = rr.Out().Write([]byte("LISTING\n"))
		},
	)
}

func TestTheShellOptionLetterTakesTheNextWordAndNotTheScript(t *testing.T) {
	for _, tc := range []struct {
		name   string
		letter string
		argv   []string
		moved  []string
		listed []bool
		out    string
		code   int
	}{
		// The bug's own shape. With the letter named, the word after it is
		// the option's name and the command string still runs; the row below
		// it is the same argument vector with no letter named, where the word
		// is a script operand nobody can open. The two answers are the whole
		// point of the pair: a front end that quietly took the word either
		// way, or neither way, could not tell them apart.
		{
			name: "the letter takes the word", letter: "O",
			argv:  []string{"testsh", "-O", "checkhash", "-c", "echo RAN"},
			moved: []string{"-checkhash"}, out: "RAN\n",
		},
		{
			// And a dialect naming no letter does not take the word: `-O`
			// is read as a `set` letter, which this one has not got, and
			// `checkhash` is left to be an operand. What is reported is the
			// refused *letter* and not the operand, which is the order every
			// shell in the panel has and which this front end took until
			// #3284 — it opened the operand first and answered 127 about a
			// file nobody named. The letter is unanswered in this synthetic
			// dialect, so the refusal is the substrate's own.
			name: "no letter named leaves the word an operand", letter: "",
			argv: []string{"testsh", "-O", "checkhash", "-c", "echo RAN"},
			code: 2,
		},
		// Both signs carry it, which is measured: `bash +O extglob` turns the
		// named option off.
		{
			name: "the plus sign turns it off", letter: "O",
			argv:  []string{"testsh", "+O", "checkhash", "-c", "echo RAN"},
			moved: []string{"+checkhash"}, out: "RAN\n",
		},
		// Wherever the letter sits in a bundle the word after the *word* is
		// what it takes, and the rest of the bundle goes on being read as set
		// letters. `-Ox name` and `-xO name` are one invocation, measured.
		{
			name: "at the end of a bundle", letter: "O",
			argv:  []string{"testsh", "-xO", "checkhash", "-c", "echo RAN"},
			moved: []string{"-checkhash"}, out: "RAN\n",
		},
		{
			name: "at the head of a bundle", letter: "O",
			argv:  []string{"testsh", "-Ox", "checkhash", "-c", "echo RAN"},
			moved: []string{"-checkhash"}, out: "RAN\n",
		},
		// Nothing attaches, and the two rows above are what say so: had the
		// rest of the word been read as the name, `-Ox name` would have moved
		// `x` and left `name` to be a script nobody can open. Measured on
		// bash 5.3.20 from the other side — `bash -Ocheckhash -c cmd` refuses
		// `-c` as an option name, which is the next *word* and not the
		// letters welded on.
		{
			name: "welded letters are letters", letter: "O",
			argv:  []string{"testsh", "-Oe", "checkhash", "-c", "echo RAN"},
			moved: []string{"-checkhash"}, out: "RAN\n",
		},
		// Two of them, in the order they were written.
		{
			name: "twice, last written last", letter: "O",
			argv:  []string{"testsh", "-O", "a", "+O", "b", "-c", "echo RAN"},
			moved: []string{"-a", "+b"}, out: "RAN\n",
		},
		// A name the namespace will not take ends the invocation with its
		// own status and runs nothing.
		{
			name: "a refused name runs nothing", letter: "O",
			argv:  []string{"testsh", "-O", "nosuch", "-c", "echo RAN"},
			moved: []string{"-nosuch"}, code: 2,
		},
		// Past the operands it is not an option at all, which is the
		// ordinary rule and is worth a row here because this letter reaches
		// forward for a word: the reaching must not start after the options
		// have ended. Measured — `bash -c 'echo hi' -O` prints `hi`.
		{
			name: "after the operands it is a parameter", letter: "O",
			argv:   []string{"testsh", "-c", "echo RAN", "--", "-O"},
			listed: nil, out: "RAN\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := &shellOptionRecorder{refuse: "nosuch"}
			sh := shell()
			sh.Semantics.ShellOptionInvocationLetter = tc.letter
			sh.Register = rec.install
			out, errs, code := runArgs(t, sh, tc.argv...)
			if code != tc.code {
				t.Fatalf("status %d, want %d — out %q, stderr %q", code, tc.code, out, errs)
			}
			if out != tc.out {
				t.Errorf("stdout %q, want %q", out, tc.out)
			}
			if got := strings.Join(rec.moved, " "); got != strings.Join(tc.moved, " ") {
				t.Errorf("moved %q, want %q", got, strings.Join(tc.moved, " "))
			}
		})
	}
}

// TestTheShellOptionLetterWithNoWordLists is the listing half, which needs an
// argument vector that really ends on the letter — a table row cannot hold one
// without the `-c` string that makes every other row observable.
func TestTheShellOptionLetterWithNoWordLists(t *testing.T) {
	for _, tc := range []struct {
		name       string
		argv       []string
		reissuable bool
	}{
		{"the minus lists in the plain form", []string{"testsh", "-O"}, false},
		{"the plus lists in the re-inputtable form", []string{"testsh", "+O"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := &shellOptionRecorder{}
			sh := shell()
			sh.Semantics.ShellOptionInvocationLetter = "O"
			sh.Register = rec.install
			sh.Stdin = pipeWith(t, "echo RAN\n")
			out, errs, code := runArgs(t, sh, tc.argv...)
			if code != 0 {
				t.Fatalf("status %d — out %q, stderr %q", code, out, errs)
			}
			if len(rec.listed) != 1 || rec.listed[0] != tc.reissuable {
				t.Fatalf("listed %v, want one listing with reissuable=%v", rec.listed, tc.reissuable)
			}
			if len(rec.moved) != 0 {
				t.Errorf("moved %v, want nothing moved", rec.moved)
			}
			// The listing comes first and the script still runs, which is
			// the half a front end that ended the invocation would lose.
			if want := "LISTING\nRAN\n"; out != want {
				t.Errorf("stdout %q, want %q", out, want)
			}
		})
	}
}
