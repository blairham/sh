// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// When a `$( … )` body is parsed, which this shell answers differently from
// the shell it is named after's own 3.2 build.
//
// Measured 2026-09-19 against bash 5.3.20, script files under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` with standard input on the null
// device. The `$( … )` body is read **with the line that holds it** and the
// backquoted body is read when the substitution runs:
//
//	echo before; v=$(if); echo after     nothing written, status 2
//	echo before; v=`if`; echo after      `before`, a complaint, `after`
//
// The control that makes the first row a statement about *parsing* rather
// than about how far a failure unwinds is the same body in a branch nothing
// takes — the substitution is never reached and the line is still refused.
//
// bash 3.2.57 writes `before` for both spellings, so this is a version line
// inside one lineage and not the name's answer. See
// syntax.Dialect.SubstitutionBodyRead (#2857).
func TestASubstitutionBodyIsParsedWithTheLineThatHoldsIt(t *testing.T) {
	for _, c := range []struct {
		name, src, out string
		status         int
		quiet          bool
	}{
		{
			// Nothing in front of the substitution runs.
			"the newer spelling stops the line",
			"echo before; v=$(if); echo after\n", "", 2, false,
		},
		{
			// The control. Never reached, and refused anyway, which is what
			// says the body was *parsed* rather than run.
			"a branch nothing takes stops the line too",
			"false && v=$(if); echo \"after=$?\"\n", "", 2, false,
		},
		{
			// The older spelling is not read with the line here, so the
			// `echo` in front of it runs and so does the one behind it.
			"the older spelling waits for the substitution",
			"echo before; v=`if`; echo after\n", "before\nafter\n", 0, false,
		},
		{
			"the older spelling in a branch nothing takes says nothing",
			"false && v=`if`; echo \"after=$?\"\n", "after=1\n", 0, true,
		},
		{
			// The logical line is the unit: line 1 runs before line 2 is
			// refused. The whole file is handed over at once here, so this
			// is the runner's granularity rather than a front end's.
			"the line before it still runs",
			"printf 'start\\n'\necho before; v=$(if); echo after\n",
			"start\n", 2, false,
		},
		{
			// Each body's own spelling decides, so a `$( … )` nested in a
			// `$( … )` is read with the line.
			"a nested newer body is read with the line",
			"echo before; v=$(echo $(if)); echo after\n", "", 2, false,
		},
		{
			// And the same nesting under an older opener is not, because the
			// outer body is not read until it runs.
			"a nested body under an older opener is not",
			"echo before; v=`echo $(if)`; echo after\n", "before\nafter\n", 0, false,
		},
		{
			// An expansion's operand is read with the line as well.
			"a substitution in an operand is read with the line",
			"echo before; echo \"${v-$(if)}\"; echo after\n", "", 2, false,
		},
		{
			// And a `'` in that operand keeps it out of the line's read,
			// because the scan for the closing brace never saw an opener
			// there. See
			// syntax.Dialect.AQuotedOperandHidesASubstitutionFromItsLine.
			//
			// `before` is the whole of what this row asserts: the reference
			// writes it too and then gives the line up, where a body read
			// with the line writes nothing at all. What it gives the line up
			// *with* still differs — bash 5.3.20 says `command substitution:
			// line 2:` and leaves 1, and this shell says `line 1:` and
			// leaves 2 — which is the span placement #3355 holds and was the
			// same before this change.
			"a quote in the operand hides it from the line",
			"echo before; echo \"${v-'$(if)'}\"; echo after\n",
			"before\n", 2, false,
		},
		{
			// The control for that one: the quotes moved off the
			// substitution, which is read with the line as before.
			"a quote elsewhere in the operand hides nothing",
			"echo before; echo \"${v-'a'$(if)}\"; echo after\n", "", 2, false,
		},
		{
			// Never reached, never read, nothing said — the hidden body's
			// own branch control.
			"a hidden body in a branch nothing takes is silent",
			"v=SET\necho \"${v-'$(if)'}\"\necho after\n", "SET\nafter\n", 0, true,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, status := runScript(t, scriptFile(t, c.src))
			if out != c.out || status != c.status {
				t.Errorf("ran %q: out %q status %d, want %q and %d (errs %q)",
					c.src, out, status, c.out, c.status, errs)
			}
			if c.quiet && errs != "" {
				t.Errorf("ran %q: said %q, want silence", c.src, errs)
			}
			if !c.quiet && errs == "" {
				t.Errorf("ran %q: said nothing, want a complaint", c.src)
			}
		})
	}
}

// TestABodyReadWithItsLineIsStillReadAgainWhenItRuns is what says the first
// read does not replace the second, and it is the row that decided the
// design.
//
// Measured 2026-09-19 against bash 5.3.20, one line, `shopt -s
// expand_aliases` in front of it so that a non-interactive shell expands at
// all: `alias t=echo; v=$(t hi); echo "[$v]"` answers `[hi]`. So the body
// that was read with the line is expanded against the alias table as it
// stands *afterwards* — the table the same line went on to change — and a
// tree kept from the first read would answer `[]`.
//
// dash reads it once and answers `t: not found`, which is its own column of
// `alias/nested-text-expands-where-the-command-string-did-not` and not this
// one's.
func TestABodyReadWithItsLineIsStillReadAgainWhenItRuns(t *testing.T) {
	src := "shopt -s expand_aliases\nalias t=echo; v=$(t hi); echo \"[$v]\"\n"
	out, errs, status := runScript(t, scriptFile(t, src))
	if !strings.Contains(out, "[hi]") || status != 0 {
		t.Errorf("ran %q: out %q status %d, want `[hi]` and 0 (errs %q)",
			src, out, status, errs)
	}
}
