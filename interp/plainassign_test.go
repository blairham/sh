// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// plainAssignScript assigns each shape plainAssignValue admits, beside the
// neighbors it must leave to the full road: an unset name, a produced
// parameter, an array, a reference, a value holding a separator, the integer
// and export attributes on the name written, `set -u`, a traced assignment,
// and declaration operands.
const plainAssignScript = `p() { printf '[%s]' "$@"; echo; }
t=abc; v=hello/world; n=7; sp='a b'; arr=(x yy zzz); typeset -i ti; export ex
x=0; p "$x"; x=a:b,c.d/e-f+g]; p "$x"; x=${t}lit; p "$x"; x=lit${t}; p "$x"
x=$t$n${t}; p "$x"; x=${#v}; p "$x"; x=${#v}x$t; p "$x"; x=${#un}; p "$x"
x=${un}z; p "$x"; x=${sp}z; p "$x"; x=${#sp}; p "$x"; x=${COUNT}a; p "$x"
x=${#COUNT}; p "$x"; x=${#arr}; p "$x"; x=${arr}q; p "$x"; x=${#LIST}; p "$x"
ti=${n}3; p "$ti"; ex=${t}e; p "$ex" "$(printenv ex)"
set -- one two; x=$1${2}x; p "$x"; x=${#1}; p "$x"
f() { local y=${t}loc z=${#v}; p "$y" "$z"; export w=${n}w; p "$w"; }; f
command eval 'typeset -n ref=t'; x=${ref}r; p "$x"; x=${#ref}; p "$x"
set -x; x=${t}tr; set +x; p "$x"
set -u; x=${#t}u; p "$x"; ( x=${#nope}; p "$x" ) 2>/dev/null; p "st=$?"
`

// TestAPlainAssignValueAnswersAsTheFullRoadDoes runs plainAssignScript with
// plainAssignValue on and off, and requires the same output and status (#6423).
func TestAPlainAssignValueAnswersAsTheFullRoadDoes(t *testing.T) {
	f, err := syntax.Parse(plainAssignScript, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	for name, sem := range plainParamVectors() {
		t.Run(name, func(t *testing.T) {
			run := func(on bool) (string, int) {
				was := SetPlainAssignValueShortcutForTest(on)
				defer SetPlainAssignValueShortcutForTest(was)
				var out strings.Builder
				r := plainParamRunner(t, sem, &out)
				r.Env = append(r.Env, "PATH=/usr/bin:/bin")
				st, err := r.Run(context.Background(), f)
				if err != nil {
					t.Fatalf("shortcut %v: %v", on, err)
				}
				return out.String(), st
			}
			full, fullStatus := run(false)
			short, shortStatus := run(true)
			if short != full || shortStatus != fullStatus {
				t.Errorf("with the shortcut:\n%s(status %d)\nwithout it:\n%s(status %d)",
					short, shortStatus, full, fullStatus)
			}
			// The rows the comparison leans on, so a script that stopped
			// early compares nothing.
			for _, row := range []string{
				"[0]\n", "[a:b,c.d/e-f+g]]\n", "[abclit]\n", "[litabc]\n", "[abc7abc]\n",
				"[11]\n", "[11xabc]\n", "[abce][abce]\n", "[onetwox]\n", "[3]\n",
				"[abcloc][11]\n", "[7w]\n", "[abcr]\n", "+ x=abctr\n", "[3u]\n",
			} {
				if !strings.Contains(full, row) {
					t.Errorf("the script did not reach %q:\n%s", row, full)
				}
			}
		})
	}
}

// TestThePlainAssignValueRouteIsReached says which values plainAssignValue
// answers itself, so the comparison above is known to compare the route with
// the full road rather than the full road with itself.
func TestThePlainAssignValueRouteIsReached(t *testing.T) {
	sem := plainParamVectors()["first element"]
	var out strings.Builder
	r := plainParamRunner(t, sem, &out)
	f, err := syntax.Parse("t=abc; v=hello/world; arr=(a b)\n", syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		value, want string
		taken       bool
	}{
		{"0", "0", true},
		{"${t}lit", "abclit", true},
		{"lit$t", "litabc", true},
		{"${#v}", "11", true},
		{"${#v}x${t}", "11xabc", true},
		{"~/x", "", false},
		{"a=b", "", false},
		{"'q'", "", false},
		{"${#arr}", "", false},
		{"${COUNT}", "", false},
		{"${#un}", "", false},
		{"${#v[0]}", "", false},
		{"${t:-d}", "", false},
	} {
		g, err := syntax.Parse("x="+c.value+"\n", syntax.Core())
		if err != nil {
			t.Fatalf("%s: %v", c.value, err)
		}
		a := g.Stmts[0].Expr.(*syntax.Pipeline).Cmds[0].(*syntax.SimpleCmd).Assigns[0]
		got, taken := PlainAssignValueForTest(r, a.Value)
		if taken != c.taken || got != c.want {
			t.Errorf("%s: answered %q, %v; want %q, %v", c.value, got, taken, c.want, c.taken)
		}
		// And a value the route answers is a plain store too, rather than one
		// the full road's thirty questions are asked about first.
		if c.taken && !StoresAPlainScalarForTest(r, a) {
			t.Errorf("%s: the route answers it, and the plain store does not take it", c.value)
		}
	}
}

// TestAPlainLengthCountsAsTheFullRoadDoes is `${#name}` of a multibyte value,
// which the route counts by the full road's own stringLength: a character in
// a UTF-8 locale, a byte under C. plainAssignScript cannot reach it, since its
// runner answers no locale.
func TestAPlainLengthCountsAsTheFullRoadDoes(t *testing.T) {
	const src = `x=é; LC_ALL=here.UTF-8; y=${#x}u; echo $y; LC_ALL=C; y=${#x}c; echo $y`
	for _, on := range []bool{false, true} {
		was := SetPlainAssignValueShortcutForTest(on)
		out, _ := run(t, src, func(r *Runner) {
			sem := *r.Semantics
			sem.MultibyteEncodingIsHonored = Yes
			r.Semantics = &sem
			r.LocaleCharset = fakeLocales(nil)
		})
		SetPlainAssignValueShortcutForTest(was)
		if out != "1u\n2c\n" {
			t.Errorf("shortcut %v: got %q, want %q", on, out, "1u\n2c\n")
		}
	}
}
