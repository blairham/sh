// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/syntax"
)

// What this shell says about a loop whose name is an expansion, and what it
// exits with. `n=x; for $n in a b` is refused by every shell in the panel and
// was taken here in every dialect, binding a variable literally called `n` at
// status 0 with nothing said (#1076).
//
// Measured 2026-09-06, `env -i PATH=/usr/bin:/bin` with a scratch HOME, over a
// script file and through `-c` alike. Six panel columns give the one refusal
// **four** wordings — the three bash columns share theirs — at three statuses,
// which is why the detection was left out of #1057 and filed on its own.
//
// The whole rendered report is asserted rather than a substring of it: the
// location, the sentence and whether the offending line is echoed back are
// three separate answers, and a `Contains` check passes with any two of them
// wrong.
func TestALoopNamedByAnExpansionIsRefusedInThisShellsWords(t *testing.T) {
	const src = "n=x\nfor $n in a b; do :; done\n"
	const want = "s.sh: 2: Syntax error: Bad for loop variable\n"
	_, err := syntax.Parse(src, dash.Dialect())
	if err == nil {
		t.Fatal("parsed; every shell in the panel refuses this")
	}
	d := dash.Diagnostics().ForScript()
	if got := d.ParseDiagnostic("s.sh", "", err, src); got != want {
		t.Errorf("\n got %q\nwant %q", got, want)
	}
	if got, want := d.StatusForParseError(err), 2; got != want {
		t.Errorf("status = %d, want %d", got, want)
	}
}

// This shell names no word at all: one sentence for every spelling, and for a
// name that is merely not a name. So a wording that quoted the offending word
// would be wrong here in the direction a reader cannot check — it would look
// more helpful than the shell is.
func TestThisShellNamesNoWord(t *testing.T) {
	const want = "Syntax error: Bad for loop variable"
	for _, name := range []string{"$n", "${n}", `"$n"`, "$(echo n)", "1x", `"i"`} {
		src := "for " + name + " in a b; do :; done\n"
		_, err := syntax.Parse(src, dash.Dialect())
		if err == nil {
			t.Fatalf("%q parsed; this shell refuses it", src)
		}
		if got := dash.Diagnostics().ParseFailure(err); got != want {
			t.Errorf("%q:\n got %q\nwant %q", src, got, want)
		}
	}
}

// A *quoted* name is the axis beside it, and this shell is on the refusing
// side: `for "i" in a b` is refused here where one shell in the panel removes
// the quoting and binds `i`. The escape is refused with the quotes.
func TestAQuotedLoopNameIsRefusedHere(t *testing.T) {
	if dash.Dialect().ForNameMayBeQuoted {
		t.Error("this shell wants a loop name written plainly")
	}
	for _, name := range []string{`"i"`, `'i'`, "i\"\"", `"i"x`, `\i`} {
		src := "for " + name + " in a b; do :; done\n"
		if _, err := syntax.Parse(src, dash.Dialect()); err == nil {
			t.Errorf("%q parsed; this shell refuses it", src)
		}
	}
}
