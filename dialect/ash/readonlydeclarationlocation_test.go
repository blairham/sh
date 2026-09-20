// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"
)

// A readonly refusal raised by an assignment *inside* a declaration builtin
// names that builtin in the location, where the same refusal raised by a bare
// assignment names nothing (#3908).
//
// This is the residue of #2761 on one route rather than a reopening of it:
// `cd`, `unset`, `exec` and the rest already reach
// Diagnostics.BuiltinLocation correctly. What did not is the refusal raised
// by an assignment a declaration builtin carried, because it reports as an
// assignment rather than as the builtin that carried it —
// Runner.reportReadonlyRefusal puts the builtin aside for the report unless
// the dialect names it for this very refusal, and this column listed only
// `getopts` and `read`.
//
// Measured 2026-09-20, BusyBox v1.37.0 in the digest-pinned alpine image,
// `env -i PATH=/usr/bin:/bin LC_ALL=C /bin/sh case.sh` over a script whose
// first line is `readonly q=1`:
//
//	readonly q=2            case.sh: readonly: line 2: q: is read only
//	export   q=3            case.sh: export: line 2: q: is read only
//	f(){ local q=5; }; f    case.sh: local: line 2: q: is read only
//	q=4                     case.sh: line 2: q: is read only
//
// The wording is the plain one in all four — the builtin is never in the
// *sentence* here, which is dash's placement and not this shell's — so
// Diagnostics.ReadonlyVariableInDeclaration stays the bare `%[1]s: is read
// only` and only ReadonlyRefusalNamesBuiltin moves.
//
// The line *number* is deliberately not asserted. These rows run under `-c`,
// and BusyBox counts a `-c` script's lines from the line after the first —
// `unset q` on line 2 is `line 1` there — where the script-file route above
// agrees with ours byte for byte. That drift is a separate question from
// whether the builtin reaches the location at all, which is this one.
func TestAReadonlyRefusalThroughADeclarationNamesTheBuiltin(t *testing.T) {
	for _, tc := range []struct{ name, src, segment, why string }{
		{
			"readonly",
			"readonly q=1\nreadonly q=2\n", ": readonly: line ",
			"the spelling the freeze was written with is also the one that names itself",
		},
		{
			"export",
			"readonly q=1\nexport q=3\n", ": export: line ",
			"the second POSIX declaration utility, same placement",
		},
		{
			"local, from inside a function",
			"readonly q=1\nf(){ local q=5; }\nf\n", ": local: line ",
			"the refusal is the declaration's own, so the builtin is named from inside a call too",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runIn(t, tc.src)
			if !strings.Contains(out, tc.segment) {
				t.Errorf("got %q, want it to contain %q — %s", out, tc.segment, tc.why)
			}
			if want := "q: is read only"; !strings.Contains(out, want) {
				t.Errorf("got %q, want it to contain %q — the sentence is the plain "+
					"one and the builtin is in the location beside it", out, want)
			}
			if st != 2 {
				t.Errorf("got status %d, want 2", st)
			}
		})
	}
}

// The bare row, and this comment is the honest label it needs: it is a **pin,
// not a control**. A bare assignment reaches the refusal with no builtin
// running, so nothing this change could have got wrong can put a name in its
// location — the row records the measured line and cannot be made to fail by
// mutating the table above. The mutation that does discriminate is dropping
// the three spellings from Diagnostics.ReadonlyRefusalNamesBuiltin, which
// takes every row of the test above with it.
func TestABareAssignmentToAFrozenNameNamesNoBuiltin(t *testing.T) {
	out, st := runIn(t, "readonly q=1\nq=4\necho survived\n")
	if want := "q: is read only"; !strings.Contains(out, want) {
		t.Errorf("got %q, want it to contain %q", out, want)
	}
	for _, word := range []string{"readonly:", "export:", "local:", "line "} {
		if strings.Contains(out, word) {
			t.Errorf("got %q, want no %q in it — the store refused this, not a "+
				"builtin, and BusyBox writes neither a builtin nor a line here", out, word)
		}
	}
	if st != 2 {
		t.Errorf("got status %d, want 2 — the refusal ends the script either way", st)
	}
}
