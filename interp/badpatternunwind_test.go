// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// badPatternSem is the vector a refused pattern needs: the policy that calls
// an unterminated bracket a bad pattern, a fatal status to end at, and the
// boundary answer that makes text a special builtin is running catchable.
func badPatternSem() Semantics {
	s := permissive()
	s.UnterminatedBracket = BracketBadPattern
	s.FatalErrorStatusIsOne = Yes
	s.FatalErrorEndsBorrowedTextOnly = Yes
	return s
}

// A refused pattern is an **error** the shell gave up over, not a request to
// stop, so the boundary a special builtin draws around text it is running
// catches it exactly as it catches every other fatal error. This raised an
// unannotated stop, so an `eval` around a bad pattern abandoned the whole
// script and everything after the first one was lost (#3398).
func TestARefusedPatternIsAnErrorTheBorrowedTextBoundaryCatches(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		stopped         bool
	}{
		{
			name: "an eval catches it and the script carries on",
			src:  `eval 'case "[a" in ([) echo one;; (*) echo two;; esac'; echo "st=$?"; echo after`,
			want: "st=1\nafter\n",
		},
		{
			name: "a second pattern after it still runs",
			src: `t() { eval "case \"\$2\" in ($1) printf Y;; *) printf n;; esac"; }; ` +
				`for p in '[' 'a'; do printf '%s:' "$p"; t "$p" '[a'; printf ' '; done; echo; echo still-here`,
			want: " a:n \nstill-here\n",
		},
		{
			name:    "a function body is not a boundary",
			src:     `f() { case '[a' in ([) echo one;; esac; }; f; echo after`,
			stopped: true,
		},
		{
			name:    "and neither is the top level",
			src:     `case '[a' in ([) echo one;; esac; echo after`,
			stopped: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, withSem(badPatternSem()))
			if !strings.Contains(out, "bad pattern: [") {
				t.Errorf("got %q, want the refusal in it", out)
			}
			rest := out[strings.Index(out, "bad pattern: [")+len("bad pattern: ["):]
			if tc.stopped {
				if strings.Contains(rest, "after") {
					t.Errorf("got %q, want the script given up", out)
				}
				if st != 1 {
					t.Errorf("status = %d, want the dialect's fatal 1", st)
				}
				return
			}
			if !strings.HasSuffix(out, tc.want) {
				t.Errorf("got %q, want it to end with %q", out, tc.want)
			}
			if st != 0 {
				t.Errorf("status = %d, want 0 — the script ran past it", st)
			}
		})
	}
}

// The boundary answer is what decides it, so the dialect that does not draw
// one round borrowed text still loses the script. Without this row the catch
// above could be a rule rather than an axis being read.
func TestARefusedPatternEndsTheScriptWhereBorrowedTextIsNoBoundary(t *testing.T) {
	sem := badPatternSem()
	sem.FatalErrorEndsBorrowedTextOnly = No
	out, st := run(t, `eval 'case "[a" in ([) echo one;; esac'; echo after`, withSem(sem))
	if !strings.Contains(out, "bad pattern: [") {
		t.Errorf("got %q, want the refusal in it", out)
	}
	if strings.Contains(out, "after") {
		t.Errorf("got %q, want the script given up", out)
	}
	if st != 1 {
		t.Errorf("status = %d, want 1", st)
	}
}

// A condition carries its own number, which is the one row the fatal status
// does not decide: measured 2026-09-18, `[[ '[a' == [ ]]` ends real zsh at 2
// where the `case` spelling of the same pattern ends it at 1.
func TestARefusedPatternInAConditionCarriesTheConditionsStatus(t *testing.T) {
	sem := badPatternSem()
	out, st := run(t, `[[ '[a' == [ ]]; echo after`, withSem(sem))
	if !strings.Contains(out, "bad pattern: [") {
		t.Errorf("got %q, want the refusal in it", out)
	}
	if strings.Contains(out, "after") {
		t.Errorf("got %q, want the script given up", out)
	}
	if st != 2 {
		t.Errorf("status = %d, want the condition's 2", st)
	}
}
