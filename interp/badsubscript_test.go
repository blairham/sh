// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A subscript is an expression, so one that will not read is the failure
// `$((b c))` is: reported, and the command given up on.
//
// It was an error nobody read. Every path took the value and dropped the
// error beside it, so `${a[b c]}` expanded to nothing at status 0 and the
// script carried on — and an empty string is a plausible value for a real
// element, so nothing downstream could tell.
func TestABadSubscriptIsReportedWhereverOneIsWritten(t *testing.T) {
	for _, src := range []string{
		// Reading an element.
		`a=(x y z); echo "[${a[b c]}]"; echo after`,
		// The same text refused by the parser rather than the evaluator.
		`a=(x y z); echo "[${a[1+]}]"; echo after`,
		// The length operator, which reaches the element through the same
		// reading and answered 0 — a plausible length.
		`a=(x y z); echo "[${#a[b c]}]"; echo after`,
		// An operator that reaches an element.
		`a=(x y z); echo "[${a[b c]:-d}]"; echo after`,
		// Assigning through one.
		`a=(x y z); echo "[${a[b c]:=v}]"; echo after`,
		// A substring's offset and its length, which are the same reading
		// reached by another spelling.
		`x=abcdef; echo "[${x:b c:2}]"; echo after`,
		`x=abcdef; echo "[${x:2:b c}]"; echo after`,
	} {
		out, st := runBadSubscript(t, src)
		if !strings.Contains(out, "b c") && !strings.Contains(out, "1+") {
			t.Errorf("%s: output %q names neither expression", src, out)
		}
		if strings.Contains(out, "after") {
			t.Errorf("%s: output %q ran on past the failure", src, out)
		}
		if st == 0 {
			t.Errorf("%s: status 0, want a failure", src)
		}
	}
}

// Writing an element names it by expression too, and the failure ends the
// script. It had a wording of its own that named the array rather than the
// expression, and it went on to the next command — so an array a script
// thought it had written was untouched and nothing stopped.
func TestABadSubscriptInAnAssignmentEndsTheScript(t *testing.T) {
	out, st := runBadSubscript(t, `a=(x y z); a[1+]=v; printf "[%s]" "${a[@]}"; echo " after"`)
	if !strings.Contains(out, "1+") {
		t.Errorf("output %q does not name the expression", out)
	}
	if strings.Contains(out, "after") {
		t.Errorf("output %q ran on past the failure", out)
	}
	if st == 0 {
		t.Errorf("status 0, want a failure")
	}
}

// `unset` is where the panel divides, and it divides three ways rather than
// two. The rows are run over a *pair of lines* on purpose: a give-up that
// takes the rest of the line with it and a give-up that ends the script print
// the same nothing on one line, which is how bash came to be recorded as
// ending a script it runs to the end (#3485).
func TestABadSubscriptToUnsetGivesUpAsMuchAsTheDialectDoes(t *testing.T) {
	const src = "a=(x y z)\n" +
		`unset "a[1+]"; echo "same=$? n=${#a[@]}"` + "\n" +
		`echo "next=$?"`
	for _, c := range []struct {
		name   string
		giveUp BadSubscriptPolicy
		want   []string
		absent []string
	}{
		// The array is untouched under all three: the element named was
		// never resolved, so there was nothing to take away.
		{
			"a failed builtin", BadSubscriptReported,
			[]string{"same=1 n=3", "next=0"},
			nil,
		},
		{
			"the command and its line", BadSubscriptAbandonsTheCommand,
			[]string{"next=1"},
			[]string{"same="},
		},
		{
			"the script", BadSubscriptEndsTheScript,
			nil,
			[]string{"same=", "next="},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runGrammar(t, src, nil, func(r *Runner) {
				sem := *r.Semantics
				sem.BadSubscriptToUnset = c.giveUp
				// The give-up takes the dialect's own fatal status, so the
				// row has to answer that axis or it would be asserting on
				// the core's refusal instead of on the give-up.
				sem.FatalErrorStatusIsOne = Yes
				r.Semantics = &sem
			})
			if !strings.Contains(out, "1+") {
				t.Errorf("output %q does not name the expression", out)
			}
			for _, want := range c.want {
				if !strings.Contains(out, want) {
					t.Errorf("output %q is missing %q", out, want)
				}
			}
			for _, absent := range c.absent {
				if strings.Contains(out, absent) {
					t.Errorf("output %q ran %q, which this answer gives up", out, absent)
				}
			}
		})
	}
}

// And the answer that gives up a command gives up a **command string** whole,
// which is the one place it and the fatal answer coincide. Measured on bash,
// where a script file resumes at the next command and `-c` does not.
func TestGivingUpACommandGivesUpACommandStringWhole(t *testing.T) {
	const src = "a=(x y z)\n" + `unset "a[1+]"` + "\n" + `echo "next=$?"`
	out, st := runGrammar(t, src, nil, func(r *Runner) {
		sem := *r.Semantics
		sem.BadSubscriptToUnset = BadSubscriptAbandonsTheCommand
		r.Semantics = &sem
		r.Route = RouteCommandString
	})
	if strings.Contains(out, "next=") {
		t.Errorf("output %q ran on past the failure", out)
	}
	if st == 0 {
		t.Errorf("status 0, want a failure")
	}
}

// An axis nobody answered is refused by name rather than guessed.
func TestABadSubscriptToUnsetRefusesAnUnspecifiedAxis(t *testing.T) {
	out, _ := runGrammar(t, `a=(x y z); unset "a[1+]"; echo "st=$?"`, nil, func(r *Runner) {
		sem := *r.Semantics
		sem.BadSubscriptToUnset = BadSubscriptUnspecified
		r.Semantics = &sem
	})
	if !strings.Contains(out, "no dialect was chosen") {
		t.Errorf("output = %q, want a refusal naming the axis", out)
	}
	if got := lastLine(out); got != "st=2" {
		t.Errorf("got %q, want %q", got, "st=2")
	}
}

// A subscript that reads is untouched by any of it, which is the half a fix
// like this is likeliest to break: the reporting must be reached only by a
// subscript that actually failed.
func TestASubscriptThatReadsIsUnaffected(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`a=(x y z); echo "[${a[1+1]}]"`, "[z]"},
		{`a=(x y z); i=1; echo "[${a[i+1]}]"`, "[z]"},
		{`a=(x y z); echo "[${a[k]}]"`, "[x]"},
		{`x=abcdef; echo "[${x:1+1:2}]"`, "[cd]"},
		{`a=(x y); a[1+1]=Q; printf "[%s]" "${a[@]}"`, "[x][y][Q]"},
		{`a=(x y z); unset "a[1+1]"; printf "[%s]" "${a[@]}"`, "[x][y]"},
	} {
		out, st := runBadSubscript(t, c.src)
		if got := strings.TrimSpace(out); got != c.want {
			t.Errorf("%s = %q, want %q", c.src, got, c.want)
		}
		if st != 0 {
			t.Errorf("%s: status = %d, want 0", c.src, st)
		}
	}
}

// runBadSubscript runs src with the `unset` give-up axis answered, so a case
// that does not turn on it is not refused for want of it.
func runBadSubscript(t *testing.T, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, nil, func(r *Runner) {
		sem := *r.Semantics
		sem.BadSubscriptToUnset = BadSubscriptEndsTheScript
		r.Semantics = &sem
	})
}
