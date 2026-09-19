// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

const printBadUsage = "Usage: print [-enprsvC] [-f format] [-u fd] [string ...]"

// TestPrintNamesEveryBadOptionOfTheRun is #3573's remaining row.
//
// This shell names **every** letter of an option run it does not have, and
// `print` was naming the first. `Semantics.BuiltinReportsEveryBadOption` is
// the answer and it was not reaching this builtin, which is the dialect's own
// and does not go through Runner.refuseOption.
//
// Measured 2026-09-18 against ksh93u+ 2012-08-01, `env -i
// PATH=/usr/bin:/bin LC_ALL=C ksh -c <probe>`, newlines shown as `|`:
//
//	print -qz        -q | -z | the usage        2
//	print -q -z      -q | -z | the usage        2  — the run, not the word
//	print -nqz x     -q | -z | the usage        2  — a letter it has is stepped over
//	print -qnz x     the same
//	print -q9z       -q | -9 | -z | the usage   2  — a digit is an ordinary letter
//	print "-n y"     `- ` | -y | the usage      2  — the issue's own row
//	print -qfFMT x   -q | the usage             2  — an argument ends the run
//	print -qu3 x     -q | the usage             2
//	print -z foo     -z | the usage             2  — one letter reads the same either way
//
// So the unit is the option **run** and not the word, the walk steps over a
// letter the builtin has, and the one thing that ends it early is a letter
// that takes an argument — which is the rule interp's own everyBadOption
// already follows for the builtins that reach it.
func TestPrintNamesEveryBadOptionOfTheRun(t *testing.T) {
	for _, c := range []struct {
		src  string
		want []string
	}{
		{`print -qz`, []string{"print: -q: unknown option", "print: -z: unknown option"}},
		{`print -q -z`, []string{"print: -q: unknown option", "print: -z: unknown option"}},
		{`print -nqz x`, []string{"print: -q: unknown option", "print: -z: unknown option"}},
		{`print -qnz x`, []string{"print: -q: unknown option", "print: -z: unknown option"}},
		{`print -q9z`, []string{
			"print: -q: unknown option", "print: -9: unknown option", "print: -z: unknown option",
		}},
		{`print "-n y"`, []string{"print: - : unknown option", "print: -y: unknown option"}},
	} {
		out, st := runKsh(t, t.TempDir(), c.src)
		if st != 2 {
			t.Errorf("%s: reported %d, want 2", c.src, st)
		}
		for _, w := range c.want {
			if !strings.Contains(out, w) {
				t.Errorf("%s: said %q, want %q in it", c.src, out, w)
			}
		}
		if n := strings.Count(out, "Usage: print"); n != 1 {
			t.Errorf("%s: said %q, want exactly one usage line, got %d", c.src, out, n)
		}
		if !strings.HasSuffix(out, printBadUsage+"\n") {
			t.Errorf("%s: said %q, want the usage after the lot", c.src, out)
		}
	}
	// A letter this builtin has and that takes an argument ends the run, so
	// only what came before it is named.
	for _, src := range []string{`print -qfFMT x`, `print -qu3 x`} {
		out, st := runKsh(t, t.TempDir(), src)
		if st != 2 || !strings.Contains(out, "print: -q: unknown option") {
			t.Errorf("%s: said %q at %d, want -q named at 2", src, out, st)
		}
		if strings.Contains(out, "-F") || strings.Contains(out, "-3") {
			t.Errorf("%s: said %q, want the argument left out of the complaints", src, out)
		}
	}
	// And one bad letter reads the same as it always did.
	out, st := runKsh(t, t.TempDir(), `print -z foo`)
	if st != 2 || strings.Count(out, "unknown option") != 1 {
		t.Errorf("print -z foo: said %q at %d, want one complaint at 2", out, st)
	}
}

// TestPrintAnswersABadOptionBeforeItsOwnRefusals is the ordering the same run
// measured, and it is this shell's own refusals that give way.
//
// `-v`, `-C` and a `-p` with no coprocess are refusals this engine makes and
// ksh93 does not — the first two are options it really has, and the third is
// a fault it reports about a coprocess. Measured 2026-09-18:
//
//	print -qv x   print: -q: unknown option + the usage   2
//	print -vq x   the same, whichever order they are written in
//	print -qC x   the same
//	print -qp x   the same
//
// So a word the shell cannot read at all is answered in front of anything it
// merely cannot do, and this engine answered its own refusal first — which
// hid a complaint the reference makes.
func TestPrintAnswersABadOptionBeforeItsOwnRefusals(t *testing.T) {
	for _, src := range []string{`print -qv x`, `print -vq x`, `print -qC x`, `print -qp x`} {
		out, st := runKsh(t, t.TempDir(), src)
		if st != 2 {
			t.Errorf("%s: reported %d, want 2", src, st)
		}
		if !strings.Contains(out, "print: -q: unknown option") {
			t.Errorf("%s: said %q, want the bad letter named", src, out)
		}
		if strings.Contains(out, "not implemented") || strings.Contains(out, "query process") {
			t.Errorf("%s: said %q, want this shell's own refusal held back", src, out)
		}
	}
	// With no bad letter in the run they are unchanged.
	if out, st := runKsh(t, t.TempDir(), `print -v x`); st != 2 ||
		!strings.Contains(out, "print: -v is not implemented yet") {
		t.Errorf("print -v x: said %q at %d, want the not-implemented refusal", out, st)
	}
	if out, st := runKsh(t, t.TempDir(), `print -p x`); st != 1 ||
		!strings.Contains(out, "print: no query process [Bad file descriptor]") {
		t.Errorf("print -p x: said %q at %d, want the coprocess refusal", out, st)
	}
}
