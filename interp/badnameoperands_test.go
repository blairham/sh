// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// runBadName runs src with the bad-name refusal fatal, which is the only
// state in which the question below can be asked at all: where the refusal is
// survivable the loop reaches every operand by carrying on.
//
// The names are read back from an EXIT trap rather than from the next line.
// A fatal refusal ends the script, so a reader written after it never runs —
// which is how "nothing was declared" and "the script stopped" look identical
// from the outside, and why this went unmeasured.
func runBadName(t *testing.T, src string, after Answer) (out string, status int) {
	t.Helper()
	return run(t, src, func(r *Runner) {
		s := permissive()
		s.BadNameToDeclarationFatal = Yes
		s.BadNameToUnsetFatal = Yes
		s.BadNameDeclaresTheOperandsAfterIt = after
		s.DeclarationNameOperands = PlainNamesOnly
		s.UnsetNameOperands = PlainNamesOnly
		s.DeclaredNameWithoutValueIsEmpty = No
		r.Semantics = &s
	})
}

const readBack = `trap 'echo "[${ok1-U}][${ok2-U}]"' EXIT; `

// A fatal refusal keeps the operands that were names: it used to hand back
// none of them, so a declaration with one bad operand among good ones
// declared nothing at all (#1211).
func TestAFatalBadNameStillDeclaresTheNamesInFrontOfIt(t *testing.T) {
	out, status := runBadName(t, readBack+`export ok1=1 ":" ok2=2; echo NOT-FATAL`, No)
	if want := "[1][U]\n"; !strings.HasSuffix(out, want) {
		t.Errorf("export ok1=1 \":\" ok2=2 = %q (status %d), want it to end %q", out, status, want)
	}
	if status == 0 {
		t.Errorf("status %d, want the refusal to have failed", status)
	}
}

// And, where the dialect says so, the ones behind it as well — which is the
// axis, and the reason the *position* of the bad name is what the measurement
// varied.
func TestWhetherAFatalBadNameDeclaresTheOperandsBehindIt(t *testing.T) {
	for _, tc := range []struct {
		name  string
		src   string
		after Answer
		want  string
	}{
		{"bad first, kept", `export ":" ok1=1 ok2=2`, Yes, "[1][2]\n"},
		{"bad first, dropped", `export ":" ok1=1 ok2=2`, No, "[U][U]\n"},
		{"bad middle, kept", `export ok1=1 ":" ok2=2`, Yes, "[1][2]\n"},
		{"bad middle, dropped", `export ok1=1 ":" ok2=2`, No, "[1][U]\n"},
		// The control: with nothing behind the refusal the two answers are
		// the same, so a probe that only ever put the bad name last could not
		// have found the axis.
		{"bad last, kept", `export ok1=1 ok2=2 ":"`, Yes, "[1][2]\n"},
		{"bad last, dropped", `export ok1=1 ok2=2 ":"`, No, "[1][2]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := runBadName(t, readBack+tc.src+`; echo NOT-FATAL`, tc.after)
			want := "sh: export: `:': not a valid identifier\n" + tc.want
			if out != want {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, status, want)
			}
		})
	}
}

// The operands behind the refusal are collected in silence: one diagnostic is
// what every column that stops writes, however many bad names follow.
func TestAFatalBadNameReportsOnlyTheFirst(t *testing.T) {
	out, _ := runBadName(t, `export ":" ok1=1 "1x" ok2=2`, Yes)
	if want := "sh: export: `:': not a valid identifier\n"; out != want {
		t.Errorf("two bad names = %q, want %q", out, want)
	}
}

// The script still stops. The give-up is held back only for as long as the
// builtin needs to declare what it was given.
func TestAFatalBadNameStillEndsTheScript(t *testing.T) {
	for _, after := range []Answer{Yes, No} {
		out, status := runBadName(t, `export ok1=1 ":" ok2=2; echo NOT-FATAL`, after)
		if want := "sh: export: `:': not a valid identifier\n"; out != want || status == 0 {
			t.Errorf("with the axis %v = %q (status %d), want %q and a failure", after, out, status, want)
		}
	}
}

// Every builtin on the shared name check, because the fix is in the check and
// each of them had to be taught to run its loop before the give-up.
func TestEveryBuiltinOnTheNameCheckKeepsItsGoodOperands(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"typeset", `typeset ok1=1 ":" ok2=2`, "[1][2]\n"},
		{"readonly", `readonly ok1=1 ":" ok2=2`, "[1][2]\n"},
		{"export", `export ok1=1 ":" ok2=2`, "[1][2]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := runBadName(t, readBack+tc.src, Yes)
			if !strings.HasSuffix(out, tc.want) || status == 0 {
				t.Errorf("%s = %q (status %d), want it to end %q and fail", tc.src, out, status, tc.want)
			}
		})
	}
}

// `unset` is the same check over the builtin that removes rather than
// declares, and the same answer: a fatal refusal takes away the names it was
// given first.
func TestAFatalBadNameToUnsetStillRemovesTheNames(t *testing.T) {
	out, status := runBadName(t,
		`ok1=1 ok2=2; `+readBack+`unset ok1 ":" ok2; echo NOT-FATAL`, Yes)
	if want := "[U][U]\n"; !strings.HasSuffix(out, want) || status == 0 {
		t.Errorf("unset ok1 \":\" ok2 = %q (status %d), want it to end %q and fail", out, status, want)
	}
}

// A refusal that is *not* fatal never asks the axis: the loop reaches every
// operand by carrying on, which is what bash does.
func TestASurvivableBadNameDeclaresEverythingWithoutAskingTheAxis(t *testing.T) {
	out, status := run(t, `export ok1=1 ":" ok2=2; echo "[$ok1][$ok2] st=$?"`, func(r *Runner) {
		s := permissive()
		s.BadNameToDeclarationFatal = No
		s.DeclarationNameOperands = PlainNamesOnly
		r.Semantics = &s
	})
	if want := "sh: export: `:': not a valid identifier\n[1][2] st=1\n"; out != want || status != 0 {
		t.Errorf("survivable refusal = %q (status %d), want %q at 0", out, status, want)
	}
}

// A subscripted operand the dialect will not take reaches the refusal by a
// different door in the same check, and answers the axis the same way.
func TestASubscriptedOperandRefusedAmongNamesReadsTheSameAxis(t *testing.T) {
	for _, tc := range []struct {
		after Answer
		want  string
	}{{Yes, "[1][2]\n"}, {No, "[1][U]\n"}} {
		out, status := runBadName(t,
			readBack+`export ok1=1 "a[1]=v" ok2=2; echo NOT-FATAL`, tc.after)
		if !strings.HasSuffix(out, tc.want) || status == 0 {
			t.Errorf("with the axis %v = %q (status %d), want it to end %q and fail",
				tc.after, out, status, tc.want)
		}
	}
}

// A bad operand behind the refusal is dropped rather than declared, which the
// environment is what can see: a name nothing in the language can read back is
// still a name a child process gets handed.
func TestABadOperandBehindTheRefusalIsNotExported(t *testing.T) {
	out, _ := runBadName(t,
		`trap '/usr/bin/env | /usr/bin/grep -c "^1x=" || true' EXIT; `+
			`export ":" ok1=1 "1x=9"`, Yes)
	if want := "0\n"; !strings.HasSuffix(out, want) {
		t.Errorf("a bad operand behind the refusal = %q, want it to end %q", out, want)
	}
}
