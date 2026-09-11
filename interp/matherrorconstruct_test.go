// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A math complaint raised by a *command* names its expression, and names the
// construct where the dialect does — #1985.
//
// The expansion route already put the expression back and the two command
// routes did not, so `(( 1/0 ))` and `let '1/0'` said `division by 0` with
// nothing to say which expression or which loop iteration had done it.
func TestAMathComplaintFromACommand(t *testing.T) {
	naming := func() Diagnostics {
		return Diagnostics{ArithError: "%[1]s: %[2]s"}
	}
	setup := func(d Diagnostics) func(*Runner) {
		s := testSemantics()
		return func(r *Runner) {
			r.Semantics = &s
			r.Diagnostics = &d
		}
	}
	t.Run("(( )) names the expression", func(t *testing.T) {
		out, st := run(t, `(( 1/0 ))`, setup(naming()))
		if st == 0 || !strings.Contains(out, " 1/0 :") {
			t.Errorf("= %q status %d, want the expression as the construct held it", out, st)
		}
	})
	t.Run("let names the expression", func(t *testing.T) {
		out, st := run(t, `let '1/0'`, setup(naming()))
		if st == 0 || !strings.Contains(out, "1/0:") {
			t.Errorf("= %q status %d, want the expression named", out, st)
		}
	})
	t.Run("a for header names the part that failed", func(t *testing.T) {
		// Three parts, and a bare reason cannot say which of them it was.
		out, _ := run(t, `for (( i=0; i<1/0; i++ )); do echo body; done`, setup(naming()))
		if strings.Contains(out, "body") {
			t.Errorf("= %q, want the loop stopped", out)
		}
		if !strings.Contains(out, "i<1/0") {
			t.Errorf("= %q, want the failing part named", out)
		}
	})
	t.Run("the expression is quoted as the construct held it", func(t *testing.T) {
		// Not trimmed at both ends, which is what this was: the shells that
		// quote an expression back quote the spaces with it, and the one
		// that does not skips the *leading* blanks alone.
		out, _ := run(t, `((    1/0   ))`, setup(naming()))
		if !strings.Contains(out, "    1/0   :") {
			t.Errorf("= %q, want the text as written", out)
		}
	})
	t.Run("a dialect that skips the leading space", func(t *testing.T) {
		d := naming()
		d.ArithErrorSkipsLeadingSpace = true
		out, _ := run(t, `((    1/0   ))`, setup(d))
		if !strings.Contains(out, "1/0   :") || strings.Contains(out, "    1/0") {
			t.Errorf("= %q, want the leading blanks dropped and the rest kept", out)
		}
	})
	t.Run("a dialect that names the construct", func(t *testing.T) {
		d := naming()
		d.ArithErrorNamesTheConstruct = true
		d.ArithErrorSkipsLeadingSpace = true
		for _, c := range []struct{ src, want string }{
			{`(( 1/0 ))`, "((: 1/0 :"},
			{`for (( i=0; i<1/0; i++ )); do :; done`, "((: i<1/0:"},
			{`[[ 1/0 -eq 1 ]]`, "[[: 1/0:"},
		} {
			out, _ := run(t, c.src, setup(d))
			if !strings.Contains(out, c.want) {
				t.Errorf("%s: = %q, want %q", c.src, out, c.want)
			}
		}
	})
	t.Run("an expansion names no construct", func(t *testing.T) {
		// The half that says this is about the construct and not about
		// arithmetic: the identical failure written as an expansion carries
		// no name in the shell that names one.
		d := naming()
		d.ArithErrorNamesTheConstruct = true
		out, _ := run(t, `x=$(( 1/0 ))`, setup(d))
		if strings.Contains(out, "((:") || strings.Contains(out, "[[:") {
			t.Errorf("= %q, want no construct named", out)
		}
	})
	t.Run("a parse failure is named the same way", func(t *testing.T) {
		d := naming()
		d.ArithErrorNamesTheConstruct = true
		d.ArithErrorSkipsLeadingSpace = true
		out, _ := run(t, `(( 1+ ))`, setup(d))
		if !strings.Contains(out, "((: 1+ :") {
			t.Errorf("= %q, want the construct named on the parse route too", out)
		}
	})
}
