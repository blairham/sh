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

// `unset` is where the panel divides, so it is the axis. One answer gives up
// on the script the way a bad expression does anywhere else; the other leaves
// a failed builtin behind and runs the next command, which is the shape a
// script can test.
func TestABadSubscriptToUnsetIsFatalOrNot(t *testing.T) {
	const src = `a=(x y z); unset "a[1+]"; echo "st=$? n=${#a[@]}"`
	for _, c := range []struct {
		name  string
		fatal Answer
		tail  string
	}{
		{"fatal", Yes, ""},
		{"a failed builtin", No, "st=1 n=3"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runGrammar(t, src, nil, func(r *Runner) {
				sem := *r.Semantics
				sem.BadSubscriptToUnsetFatal = c.fatal
				r.Semantics = &sem
			})
			if !strings.Contains(out, "1+") {
				t.Errorf("output %q does not name the expression", out)
			}
			if c.tail == "" {
				if strings.Contains(out, "st=") {
					t.Errorf("output %q ran on past the failure", out)
				}
				return
			}
			// The array is untouched either way: the element named was never
			// resolved, so there was nothing to take away.
			if got := lastLine(out); got != c.tail {
				t.Errorf("got %q, want %q", got, c.tail)
			}
		})
	}
}

// An axis nobody answered is refused by name rather than guessed.
func TestABadSubscriptToUnsetRefusesAnUnspecifiedAxis(t *testing.T) {
	out, _ := runGrammar(t, `a=(x y z); unset "a[1+]"; echo "st=$?"`, nil, func(r *Runner) {
		sem := *r.Semantics
		sem.BadSubscriptToUnsetFatal = Unspecified
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

// runBadSubscript runs src with the `unset` fatality axis answered, so a case
// that does not turn on it is not refused for want of it.
func runBadSubscript(t *testing.T, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, nil, func(r *Runner) {
		sem := *r.Semantics
		sem.BadSubscriptToUnsetFatal = Yes
		r.Semantics = &sem
	})
}
