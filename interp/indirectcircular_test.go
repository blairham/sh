// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// An expansion that never reads the value must not read it, and a **circular**
// reference is where that shows: the read walks the loop and says so, once per
// read, so a pre-read nothing wanted is a warning nothing asked for.
//
// Measured 2026-09-18 on bash 5.3.20 from a script file, counting the
// warnings after a MARK so the declaration's own two are out of the way:
// `${!r}` writes **none** and refuses the indirection, while `${r#x}` writes
// three. This shell wrote one for both; the first row is the one that is
// fixable, and the count in the second is a number with no other observable
// difference behind it (#3122).
func runIndirectCircular(t *testing.T, src string) (string, int) {
	t.Helper()
	sem := namerefAimSemantics()
	// The reading where `${!v}` takes the indirection rather than answering
	// with the name, which is the reading these rows are about: the other one
	// never reads a value at all and could not tell the two answers apart.
	sem.IndirectionYieldsName = No
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.ParamIndirection = true
	}, func(r *Runner) {
		r.Semantics = &sem
		r.Diagnostics = &Diagnostics{
			NamerefCircularWarning: "warning: %[1]s: circular name reference",
			IndirectionUndeclared:  "%[1]s: invalid indirect expansion",
		}
	})
}

func TestAnIndirectionThroughACircularReferenceReadsNothing(t *testing.T) {
	const src = `r=OUTER
f() { local -n r=r; echo MARK; echo "[%s]"; }
f
`
	warningsAfterTheMark := func(src string) int {
		t.Helper()
		out, _ := runIndirectCircular(t, src)
		_, after, found := strings.Cut(out, "MARK\n")
		if !found {
			t.Fatalf("out %q never reached the mark", out)
		}
		return strings.Count(after, "circular name reference")
	}
	// The subject: the indirection answers with a *name* or refuses, and
	// neither answer is the value — so nothing walks the reference.
	if n := warningsAfterTheMark(strings.Replace(src, "%s", "${!r}", 1)); n != 0 {
		t.Errorf("${!r} walked the reference %d time(s), want none", n)
	}
	// The control, which is what makes the row above about this expansion
	// rather than about a shell that has stopped warning: an operator that
	// really does read the value still says it.
	if n := warningsAfterTheMark(strings.Replace(src, "%s", "${r#x}", 1)); n == 0 {
		t.Error("${r#x} reads the value and must still say the reference is circular")
	}
}
