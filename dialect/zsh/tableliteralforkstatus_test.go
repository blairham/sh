// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// The unpaired-table-literal refusal ends the shell either way; what #4610 is
// about is the number it leaves behind when the shell that ends is a **forked
// copy**. There it is 0, where every neighboring fatal in the same position
// leaves 1.
//
// Measured 2026-09-26 against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m` says *not a Go executable* —
// run `-f` over a script file.
//
// **The table is built so that a shell answering 0 everywhere fails it and a
// shell answering 1 everywhere fails it too.** The first block is the finding,
// the second block is three *neighbors* of the refusal in the same position —
// which is what says this is one refusal rather than a rule about
// parentheses — and the third is three ways of showing the 0 is not a status
// being carried through from somewhere.
func TestAnUnpairedTableLiteralLeavesAForkAtZero(t *testing.T) {
	for _, c := range []struct {
		name, src string
		want      string
		// refused says this row's own refusal is the unpaired-table-literal
		// one, so the row asserts the sentence as well as the status. A row
		// that only checked the number would pass for a shell that had
		// stopped refusing at all.
		refused bool
	}{
		{
			"a subshell",
			"( typeset -A h=(a 1 b) )\nprint after=$?",
			"after=0\n",
			true,
		},
		{
			"a command substitution",
			"v=$(typeset -A h=(a 1 b); print inner)\nprint after=$?",
			"after=0\n",
			true,
		},
		{
			"a function called in a subshell",
			"f() { typeset -A h=(a 1 b); }\n( f )\nprint after=$?",
			"after=0\n",
			true,
		},
		{
			"a pipeline element",
			"typeset -A h=(a 1 b) | :\nprint ps=${pipestatus[1]}",
			"ps=0\n",
			true,
		},
		{
			// The same status read through the shapes a caller actually
			// writes, so the row is about what a script sees and not about
			// one spelling of `$?`.
			"a subshell in a condition",
			"if ( typeset -A h=(a 1 b) ); then print THEN; else print ELSE; fi",
			"THEN\n",
			true,
		},
		{
			"a subshell under errexit",
			"set -e\n( typeset -A h=(a 1 b) )\nprint after=$?",
			"after=0\n",
			true,
		},

		// The neighbors. Each is a fatal of the same shell in the same
		// position, and each leaves 1 — in the reference and here.
		{
			"the mixed-literal refusal still leaves one",
			"( typeset -A h=([a]=1 b) )\nprint after=$?",
			"after=1\n",
			false,
		},
		{
			"a readonly reassignment still leaves one",
			"( readonly r=1; r=2 )\nprint after=$?",
			"after=1\n",
			false,
		},
		{
			"a bad pattern still leaves one",
			"setopt badpattern\n( print -r -- [a )\nprint after=$?",
			"after=1\n",
			false,
		},

		// And it is zero rather than whatever the copy was holding.
		{
			"a false in front of it does not become the status",
			"( false; typeset -A h=(a 1 b) )\nprint after=$?",
			"after=0\n",
			true,
		},
		{
			"an exit 3 inside it does not become the status",
			"( (exit 3); typeset -A h=(a 1 b) )\nprint after=$?",
			"after=0\n",
			true,
		},
		{
			"a false in front of the parentheses does not become the status",
			"false\n( typeset -A h=(a 1 b) )\nprint after=$?",
			"after=0\n",
			true,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _, errs := runZshSplit(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("stdout = %q, want %q (stderr %q)", out, c.want, errs)
			}
			// The refusal is still raised — a shell that had simply stopped
			// refusing would satisfy the `after=0` rows above and nothing
			// else in this file would notice.
			if c.refused && !strings.Contains(errs, "bad set of key/value pairs") {
				t.Errorf("stderr = %q, want the refusal in it", errs)
			}
		})
	}
}

// The top level is the other side of the pair, and without it the change above
// reads as "this refusal leaves 0", which is not what the reference does: a
// script that takes it ends at **1**.
func TestAnUnpairedTableLiteralStillEndsAScriptAtOne(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		{"at the top level", "typeset -A h=(a 1 b)\nprint reached"},
		{"inside a function", "f() { typeset -A h=(a 1 b); }\nf\nprint reached"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st, errs := runZshSplit(t, t.TempDir(), c.src)
			if st != 1 {
				t.Errorf("status = %d, want 1 (stderr %q)", st, errs)
			}
			if out != "" {
				t.Errorf("stdout = %q, want nothing — the shell leaves", out)
			}
			if !strings.Contains(errs, "bad set of key/value pairs") {
				t.Errorf("stderr = %q, want the refusal in it", errs)
			}
		})
	}
}
