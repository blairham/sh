// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/syntax"
)

// A line continuation between either pair of an arithmetic expansion's
// delimiters parts them here, so the construct is a command substitution
// holding a subshell.
//
// Measured 2026-09-16 on ksh93u+ 2012 from script files under `env -i
// PATH=/usr/bin:/bin LC_ALL=C`, stdin on /dev/null:
//
//	echo "[$(\⏎( 1 + 2 ))]"   1: not found, then []
//	echo "[$(( 1 + 2 )\⏎)]"   the same
//
// bash 5.3, bash 3.2 and dash answer 3 at both ends; zsh 5.9.2 answers 3 at
// the opener and parts the closer as this shell does, which is why the two
// ends are two flags.
func TestAContinuationPartsBothArithmeticDelimiters(t *testing.T) {
	d := ksh.Dialect()
	if !d.ContinuationPartsTheArithmeticOpener || !d.ContinuationPartsTheArithmeticCloser {
		t.Fatal("ksh93 parts both of an arithmetic expansion's delimiters")
	}
	for _, src := range []string{
		"echo \"[$(\\\n( 1 + 2 ))]\"",
		"echo \"[$(( 1 + 2 )\\\n)]\"",
	} {
		out, _, _ := preset.Combined(t, dialecttest.Base{}, src)
		if !strings.Contains(out, "[]") {
			t.Errorf("%q printed %q, want an empty expansion", src, out)
		}
		if !strings.Contains(out, "1: not found") {
			t.Errorf("%q said %q, want a diagnostic naming the command `1`", src, out)
		}
	}
}

// A line continuation directly behind the `((` of an arithmetic command opens
// nothing here: the two parentheses and the pair are consumed, no command is
// produced, and reading starts again from what follows.
//
// Measured 2026-09-16 on ksh93u+ 2012 from script files. The rows that *run*
// are the whole of the reading — the refusals below them are the leftovers
// being read on their own rather than a complaint about the pair, which is
// what says the two characters were consumed and discarded.
func TestAContinuationBehindAnArithmeticCommandsOpenerEndsIt(t *testing.T) {
	d := ksh.Dialect()
	if !d.ContinuationEndsTheArithmeticCommandOpener {
		t.Fatal("ksh93 opens nothing where a continuation stands behind `((`")
	}
	for _, tc := range []struct{ src, want string }{
		{"echo pre\n((\\\necho hi\necho b", "pre\nhi\nb\n"},
		{"false\n((\\\necho \"rc=$?\"", "rc=1\n"},
		// A blank in front of the backslash takes the rule away, and so does
		// anything at all: it is the pair standing *directly* behind the
		// second parenthesis.
		{"(( \\\nx = 5 )); echo \"[$x]\"", "[5]\n"},
		{"((x\\\n = 5 )); echo \"[$x]\"", "[5]\n"},
	} {
		out, _, err := preset.Combined(t, dialecttest.Base{}, tc.src)
		if err != nil || out != tc.want {
			t.Errorf("%q: out = %q, err = %v, want %q", tc.src, out, err, tc.want)
		}
	}

	// And the shapes that end in a refusal end there because of what was
	// left. The token the refusal names is this parser's own: `for ((\⏎i=0; …`
	// is `` `i' unexpected `` in that shell and `` `))' unexpected `` here,
	// both at line 2 and both at status 3, because the two parsers give up at
	// different points in the same leftover text.
	for _, src := range []string{
		"((\\\nx = 5 )); echo \"[$x]\"",
		"if ((\\\n1 )); then echo yes; fi",
		"((\\\n)); echo \"st=$?\"",
		"for ((\\\ni=0; i<1; i++)); do echo \"[$i]\"; done",
	} {
		if _, err := syntax.Parse(src, d); err == nil {
			t.Errorf("%q parsed, want it refused at what the pair left behind", src)
		}
	}
}
