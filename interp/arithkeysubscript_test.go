// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A key that is not an expression reads and writes like any other key.
//
// The reading was already right and never ran: the parser refused the
// brackets before anything could ask what kind of name they followed, so
// `$(( counts[.accept-line] ))` was `bad math expression` where both shells
// with the attribute answer with the element. The refusal fired 69 times in
// one interactive session, from a plugin counting the widgets it had rebound
// (#1875).
func TestAnAssociativeKeyNeedNotBeAnExpression(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"an absent key reads as zero", `typeset -A m; echo "$(( m[.k] ))"`, "0"},
		{"a key that is stored reads back", `typeset -A m; m[.k]=5; echo "$(( m[.k] ))"`, "5"},
		{"the key is the text and not a name", `typeset -A m; m[.k]=5; echo "$(( m[.k] + 1 ))"`, "6"},
		// Two of them in one expression, which is what says the reader
		// carried on rather than answering the first and giving up.
		{"two keys in one expression", `typeset -A m; m[.a]=1; m[.b]=2; echo "$(( m[.a] + m[.b] ))"`, "3"},
		{"a plain assignment through the key", `typeset -A m; (( m[.k] = 3 )); echo "${m[.k]}"`, "3"},
		{"a compound assignment through the key", `typeset -A m; m[.k]=1; (( m[.k] += 5 )); echo "${m[.k]}"`, "6"},
		{"postfix through the key", `typeset -A m; m[.k]=3; echo "$(( m[.k]++ )) ${m[.k]}"`, "3 4"},
		{"prefix through the key", `typeset -A m; m[.k]=3; echo "$(( ++m[.k] )) ${m[.k]}"`, "4 4"},
		// The widget name from the report, reached the way the report
		// reaches it: the parameter goes in before the expression is read,
		// so the key does not appear in the source at all.
		{"a key arriving by expansion", `typeset -A m; m[.accept-line]=7; w=.accept-line; echo "$(( m[$w] ))"`, "7"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := run(t, tc.src, nil); strings.TrimSpace(out) != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, strings.TrimSpace(out), tc.want)
			}
		})
	}
}

// A write through such a key must land on the *element*.
//
// The hazard the parser change opened: with no expression to evaluate, a
// target carrying only text looks exactly like a bare name, and the write
// path chose by asking whether there was an index. It would have set `m`
// itself and left the element alone — a wrong answer with no diagnostic, in
// the one place a diagnostic is what a script would rely on.
func TestAWriteThroughAKeyDoesNotLandOnTheName(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a plain assignment", `typeset -A m; (( m[.k] = 3 )); echo "name=[${m}] key=[${m[.k]}]"`},
		{"an increment", `typeset -A m; m[.k]=2; (( m[.k]++ )); echo "name=[${m}] key=[${m[.k]}]"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, nil)
			if strings.TrimSpace(out) != "name=[] key=[3]" {
				t.Errorf("%s = %q, want the element written and the name untouched",
					tc.src, strings.TrimSpace(out))
			}
		})
	}
}

// Where the name is not an association the subscript really is an expression,
// and a text that is not one is still refused — moved from the parser to the
// point where the name is known, which is the only place the question can be
// answered. Both shells with the attribute refuse it there too, and leave
// every element as it was.
func TestASubscriptThatIsNoExpressionIsStillRefusedOnAnIndexedName(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"reading an element", `a=(1 2 3); echo "$(( a[.k] ))"`},
		{"reading through a scalar", `s=5; echo "$(( s[.k] ))"`},
		{"writing an element", `a=(1 2 3); (( a[.k] = 9 ))`},
		{"incrementing an element", `a=(1 2 3); (( a[.k]++ ))`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := run(t, tc.src, nil)
			if status == 0 {
				t.Errorf("%s succeeded at status 0, want a refusal", tc.src)
			}
			if !strings.Contains(out, ".k") {
				t.Errorf("%s = %q, want the subscript named", tc.src, out)
			}
		})
	}
}

// A refused write leaves the array alone. The refusal has to come *before*
// the store, not after it, and only the elements can say which.
func TestARefusedKeyWriteChangesNothing(t *testing.T) {
	src := `a=(1 2 3); (( a[.k] = 9 )); echo "[${a[0]}${a[1]}${a[2]}]"`
	out, _ := run(t, src, nil)
	if !strings.Contains(out, "[123]") {
		t.Errorf("%s = %q, want the elements unchanged", src, out)
	}
}

// A name nothing declared is the other reading under which the brackets hold
// no expression, and it is already an axis: one dialect answers zero without
// reading them at all, so a key that is no expression is zero there and the
// refusal where the axis is off. The two readings have to stay separate —
// folding them would make a refusal disappear for every name that happens not
// to exist yet.
func TestAKeySubscriptOnAnUnsetNameFollowsTheAxis(t *testing.T) {
	skip := func(a Answer) func(*Runner) {
		return func(r *Runner) {
			s := *r.Semantics
			s.ArithSubscriptSkippedWhenNameUnset = a
			r.Semantics = &s
		}
	}
	src := `echo "$(( nosuchname[.k] ))"`

	if out, status := run(t, src, skip(Yes)); strings.TrimSpace(out) != "0" || status != 0 {
		t.Errorf("with the name skipped, %s = %q at %d, want 0", src, out, status)
	}
	out, status := run(t, src, skip(No))
	if status == 0 {
		t.Errorf("without the skip, %s succeeded at status 0, want a refusal", src)
	}
	if !strings.Contains(out, ".k") {
		t.Errorf("without the skip, %s = %q, want the subscript named", src, out)
	}
}
