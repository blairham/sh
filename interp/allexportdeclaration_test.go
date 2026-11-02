// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// allexportDeclRun is declRunEnv with a PATH, so `env` is a child that can be
// asked, and with the two axes this file is about answered by set.
func allexportDeclRun(t *testing.T, src string, global, local Answer) (string, string, int) {
	t.Helper()
	return declRunEnv(t, src, func(s *Semantics) {
		// The letters the rows write, the global one among them, so a
		// function can make a global at all.
		s.DeclareOptions = "aAgiprx"
		s.AllexportMarksAGlobalDeclaredWithoutAValue = global
		s.AllexportMarksALocalDeclaredWithoutAValue = local
	}, Diagnostics{}, testPATH())
}

// `set -a` reaching a name a declaration with no value brings into being,
// both answers of Semantics.AllexportMarksAGlobalDeclaredWithoutAValue and
// AllexportMarksALocalDeclaredWithoutAValue. The mark is read back the way a
// child would see it: the name is assigned after `set +a`, so only an export
// attribute the declaration left behind can put it in `env`. `x=9` is the
// control, exported under every answer.
//
// The other axis is set to the opposite answer in every row, so a mark that
// asked the wrong question fails here (#7283).
func TestAllexportMarksANameDeclaredWithoutAValue(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, src string
		local     bool
	}{
		{"a top-level typeset", `set -a; x=9; typeset g; set +a; g=5`, false},
		{"a top-level typeset with a type letter", `set -a; x=9; typeset -i g; set +a; g=5`, false},
		{"a global from inside a function", `set -a; x=9; f() { typeset -g g; }; f; set +a; g=5`, false},
		// Nothing stands under the name, which is the road emptyshadow.go
		// takes, and an unexported outer value, which is the full one.
		{"a local over nothing", `set -a; x=9; f() { local g; set +a; g=5; env | grep '^g='; }; f`, true},
		{"a local over an outer value", `g=0; set -a; x=9; f() { local g; set +a; g=5; env | grep '^g='; }; f`, true},
	} {
		for _, answer := range []Answer{Yes, No} {
			t.Run(tc.name+"/"+answer.String(), func(t *testing.T) {
				t.Parallel()
				other := Yes
				if answer == Yes {
					other = No
				}
				global, local := answer, other
				if tc.local {
					global, local = other, answer
				}
				out, errs, st := allexportDeclRun(t, tc.src+`; env | grep -E '^(x|g)='`, global, local)
				if errs != "" || !strings.Contains(out, "x=9\n") {
					t.Fatalf("out=%q errs=%q st=%d: the control did not export", out, errs, st)
				}
				if got := strings.Contains(out, "g=5"); got != (answer == Yes) {
					t.Errorf("answer %v: out=%q, exported=%v", answer, out, got)
				}
			})
		}
	}
}

// A declaration with no value over a name that already holds one, or over one
// an earlier declaration made, brings nothing into being, so `set -a` leaves it
// alone under either answer: `x=1; set -a; declare x` lists `declare -- x="1"`
// in bash 5.3.20 and `${(t)x}` is `scalar` in zsh 5.9.2 (#7283).
func TestAllexportLeavesAValuelessDeclarationOfAStandingName(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src string }{
		{"a held value", `x=1; set -a; typeset x; set +a; x=5; env | grep -c '^x='`},
		{"an earlier declaration", `typeset x; set -a; typeset x; set +a; x=5; env | grep -c '^x='`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, errs, st := allexportDeclRun(t, tc.src, Yes, Yes)
			if out != "0\n" || errs != "" || st != 1 {
				t.Errorf("out=%q errs=%q st=%d, want x left unexported", out, errs, st)
			}
		})
	}
}

// A local given a value is an assignment, and `set -a` marks it whatever the
// two axes say: `set -a; f() { local sb=1; }` lists `declare -x sb="1"` in
// bash 5.3.20, the dialect that answers No for a valueless local. Both roads
// a local's value takes, over nothing and over an outer value (#7283).
func TestAllexportMarksALocalsValue(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src, want string }{
		{"over nothing", `set -a; f() { local v=1; env | grep '^v='; }; f`, "v=1\n"},
		{"over an outer value", `v=0; set -a; f() { local v=1; env | grep '^v='; }; f`, "v=1\n"},
		{"an empty value", `set -a; f() { local v=; env | grep '^v='; }; f`, "v=\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, errs, st := allexportDeclRun(t, tc.src, No, No)
			if out != tc.want || errs != "" || st != 0 {
				t.Errorf("out=%q errs=%q st=%d, want %q", out, errs, st, tc.want)
			}
		})
	}
}

// Nothing is asked without `set -a`: the same declarations with both axes left
// unanswered run silently.
func TestADeclarationWithoutAllexportAsksNothing(t *testing.T) {
	t.Parallel()
	out, errs, st := allexportDeclRun(t, `typeset g; f() { local h; typeset -g k; }; f; echo ok`,
		Unspecified, Unspecified)
	if out != "ok\n" || errs != "" || st != 0 {
		t.Errorf("out=%q errs=%q st=%d, want no refusal", out, errs, st)
	}
}
