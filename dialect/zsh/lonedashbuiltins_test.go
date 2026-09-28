// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A lone `-` ends a builtin's option scan, in the builtins whose option
// readers are their own.
//
// `Semantics.LoneDashIsAnOption` reached the shared reader and `echo` (#5026)
// and reached none of these, so each of them went on reading options past the
// dash — or, worse, read the dash itself as the thing it was scanning for.
//
// **The discriminating shape is an option word *behind* the dash.** "Eaten and
// skipped" and "eaten and the options end" agree wherever nothing but operands
// follows it, which is every row the axis was first measured from; that
// agreement is what hid six of these for as long as it did.
//
// Measured 2026-09-28 against `/opt/homebrew/bin/zsh`, zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m` *not a Go executable*, from
// script files under `env -i PATH=/usr/bin:/bin` with a scratch `HOME`. Every
// row below is that run, and every one of them was re-run against this
// dialect's own binary through the same driver.
//
// `umask` is measured with the rest and graded in interp rather than here —
// this harness gives the runner no umask seam, so `umask` answers "this shell
// was not given a umask to read or change" whatever the dash does. See
// TestALoneDashEndsTheUmaskOptions.
func TestALoneDashEndsTheOptionsOfABuiltinWithItsOwnReader(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		want      []string // substrings the output must hold
		notWant   []string // and ones it must not
		status    int
	}{
		{
			// Two complaints and not three: the dash is eaten, `-f` and `x`
			// are both table elements that are not there.
			name: "enable names what is behind the dash and not the dash",
			src:  `enable - -f x`,
			want: []string{"no such hash table element: -f", "no such hash table element: x"},
			// The row that was wrong: the dash named as an element of its own.
			notWant: []string{"no such hash table element: -\n"},
			status:  1,
		},
		{
			// The declaration pair reads the sign through
			// SignAloneIsAnOptionWord rather than the lone-dash axis, and the
			// panel says the two halves are one answer there too: zsh and
			// ksh93 both take the sign and both end the options at it, while
			// bash never arrives because a bare sign is a *name* there. So
			// `-r` behind the dash is an operand, and not a valid one.
			name:   "typeset ends its options at a bare sign",
			src:    `typeset - -r x=1`,
			want:   []string{"not valid in this context: -r"},
			status: 1,
		},
		{
			// The plus spelling is the same answer, measured in both columns
			// that take it — which is why one field serves both.
			name:   "and at a bare plus",
			src:    `typeset + -r x=1`,
			want:   []string{"not valid in this context: -r"},
			status: 1,
		},
		{
			// And it ends a scan that has already read letters, so this is a
			// stop and not a special case of the first word.
			name:   "even after letters have been read",
			src:    `typeset -a - -x q`,
			want:   []string{"not valid in this context: -x"},
			status: 1,
		},
		{
			name:   "readonly the same",
			src:    `readonly - -p`,
			want:   []string{"not valid in this context: -p"},
			status: 1,
		},
		{
			name:   "and export, which reads the sign on its own axis",
			src:    `export - -p`,
			want:   []string{"not valid in this context: -p"},
			status: 1,
		},
		{
			// The control that keeps the rows above about the *sign*: with
			// nothing but an assignment behind it the declaration still
			// happens, which is what "ends the options" has to leave alone.
			name:   "while an assignment behind the sign is still declared",
			src:    `typeset - x=1; print -r -- "x=$x"`,
			want:   []string{"x=1"},
			status: 0,
		},
		{
			name:    "whence takes the name behind the dash",
			src:     `whence - echo`,
			want:    []string{"echo"},
			notWant: []string{"not found"},
			status:  0,
		},
		{
			// `setopt -x` is taken, so a single complaint about `-x` is the
			// dash having ended the options and `-x` having become a *name*.
			name:    "setopt reads what is behind the dash as a name",
			src:     `setopt - -x`,
			want:    []string{"no such option: -x"},
			notWant: []string{"no such option: -\n"},
			status:  1,
		},
		{
			// **And it does not become the bare command.** Measured: `setopt`
			// alone lists and `setopt -` is silent, so eating a word is not
			// the same as never having been given one.
			name:    "but setopt with the dash eaten is silent rather than a listing",
			src:     `setopt -`,
			notWant: []string{"nohashdirs"},
			status:  0,
		},
		{
			name:    "and so is unsetopt",
			src:     `unsetopt -`,
			notWant: []string{"aliases"},
			status:  0,
		},
		{
			// The count is eaten with the word, leaving `shift` its bare form.
			name:   "shift with the dash eaten shifts one",
			src:    `set -- a b c; shift -; print -r -- "rest=$*"`,
			want:   []string{"rest=b c"},
			status: 0,
		},
		{
			// And what follows really is the count, not an operand a marker
			// would have protected.
			name:   "and a count behind the dash is still the count",
			src:    `set -- a b c; shift - 2; print -r -- "rest=$*"`,
			want:   []string{"rest=c"},
			status: 0,
		},
		{
			name:    "type names what is behind the dash and not the dash",
			src:     `type - -a echo`,
			want:    []string{"-a not found", "echo is a shell builtin"},
			notWant: []string{"- not found\n"},
			status:  1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZshOnPath(t, t.TempDir(), tc.src)
			for _, w := range tc.want {
				if !strings.Contains(out, w) {
					t.Errorf("out = %q, want it to contain %q", out, w)
				}
			}
			for _, w := range tc.notWant {
				if strings.Contains(out, w) {
					t.Errorf("out = %q, want it NOT to contain %q", out, w)
				}
			}
			if st != tc.status {
				t.Errorf("status = %d, want %d", st, tc.status)
			}
		})
	}
}

// The controls: the same letters **without** a dash in front of them are read
// exactly as they were, which is what says the change is about the dash and
// not about the letters.
//
// Two of these do not agree with the reference and are recorded that way
// rather than quietly fixed: `enable -f` is not implemented here at all, and
// `setopt -x` is refused here where the reference takes it. Both are older
// than #5040 and neither moved.
func TestTheOptionLettersBehindNoDashAreUnmoved(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		{"a declaration letter still declares", `typeset -r cx=1; print -r -- "st=$?"`, "st=0", 0},
		{"type still reports the builtin", `type -a echo`, "echo is a shell builtin", 0},
		{"whence still writes the sentence", `whence -v echo`, "echo is a shell builtin", 0},
		{"shift alone still shifts one", `set -- a b c; shift; print -r -- "rest=$*"`, "rest=b c", 0},
		{"setopt still takes an option name", `setopt xtrace; print -r -- ok`, "ok", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZshOnPath(t, t.TempDir(), tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("out = %q, want it to contain %q", out, tc.want)
			}
			if st != tc.status {
				t.Errorf("status = %d, want %d", st, tc.status)
			}
		})
	}
}

// And the readers ask rather than assume: with the axis answered **no** the
// dash is an operand again, and with it unanswered the line is refused.
//
// **This is the half the rows above cannot reach**, and a mutation said so.
// Disabling `whence`'s dash arm entirely killed nothing, because with the
// arm gone the word still falls through the letter loop with no letters in it
// and is swallowed by accident — the right answer for this dialect, reached
// without asking anything. Only a dialect that answers differently can tell
// "eaten because the answer said so" from "eaten because nothing looked".
func TestTheLoneDashReadersAskRatherThanAssume(t *testing.T) {
	run := func(t *testing.T, src string, answer interp.Answer) (string, int) {
		t.Helper()
		f, err := syntax.Parse(src+"\n", zsh.Dialect())
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		sem, dg, dl := zsh.Semantics(), zsh.Diagnostics(), zsh.Dialect()
		sem.LoneDashIsAnOption = answer
		r := &interp.Runner{
			Semantics: &sem, Diagnostics: &dg,
			Stdout: &out, Stderr: &out,
			Name: "zsh", Dialect: &dl,
		}
		zsh.Apply(r)
		status, err := r.Run(context.Background(), f)
		if err != nil {
			t.Fatalf("run %q: %v", src, err)
		}
		return out.String(), status
	}
	for _, tc := range []struct {
		name, src, wantNo string
		wantNoStatus      int
	}{
		// `whence` says nothing about a name it cannot find — measured, a
		// bare `whence -` is silent at 1 — so the dash being a name shows in
		// the **status** and nowhere else. A row written against the text
		// would have passed with the dash eaten, which is the whole reason
		// this one is here.
		{"whence looks the dash up", `whence - echo`, "echo", 1},
		{"enable names the dash as a table element", `enable - -f x`, "no such hash table element: -", 1},
		{"setopt names the dash as an option", `setopt - -x`, "no such option: -", 1},
		{"shift reads the dash as a count", `shift -`, "-", 2},
	} {
		t.Run(tc.name+", answered no", func(t *testing.T) {
			out, st := run(t, tc.src, interp.No)
			if !strings.Contains(out, tc.wantNo) {
				t.Errorf("out = %q, want it to contain %q", out, tc.wantNo)
			}
			if st != tc.wantNoStatus {
				t.Errorf("status = %d, want %d", st, tc.wantNoStatus)
			}
		})
		t.Run(tc.name+", unanswered", func(t *testing.T) {
			out, st := run(t, tc.src, interp.Unspecified)
			if !strings.Contains(out, "a lone `-` given to a builtin") {
				t.Errorf("out = %q, want the unanswered axis named", out)
			}
			if st != 2 {
				t.Errorf("status = %d, want 2", st)
			}
		})
	}
}
