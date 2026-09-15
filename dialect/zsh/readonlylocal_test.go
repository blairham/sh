// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
)

// `readonly` inside a function is a *local* readonly in this shell, where
// every other member of the panel freezes the name the shell already has —
// interp.Semantics.ReadonlyDeclaresALocal, and the last of the three
// divergences share/suite/zsh/variables.tests was written around (#2316).
//
// Measured 2026-09-15 on zsh 5.9.2, `env -i` with a scratch HOME, over a
// script file. `typeset`, `declare`, `local` and `integer` were already
// local here and `readonly` was the one left out, which is why the pair
// below runs the two words on one line: a shell with the rule in five of six
// places looks exactly like a shell with the rule.
func TestReadonlyInsideAFunctionIsLocal(t *testing.T) {
	out, st := answersRun(t, `b() { readonly B=1; }; b; printf 'readonly [%s]\n' "${B-unset}"
f() { export F=1; }; f; printf 'export   [%s]\n' "${F-unset}"`)
	want := "readonly [unset]\nexport   [1]\n"
	if out != want || st != 0 {
		t.Errorf("readonly and export from a function = %q status %d, want %q", out, st, want)
	}
}

// The consequence a script meets, and the reason it bites harder than a
// leaked name: writing to a readonly ends a non-interactive shell here, so
// without the scope a function that declares one is callable exactly once.
func TestAFunctionDeclaringAReadonlyIsCallableTwice(t *testing.T) {
	out, st := answersRun(t, `rf() { readonly RF=fixed; printf 'inside [%s]\n' "$RF"; }
rf
rf
printf 'after [%s]\n' "${RF-unset}"`)
	want := "inside [fixed]\ninside [fixed]\nafter [unset]\n"
	if out != want || st != 0 {
		t.Errorf("two calls = %q status %d, want %q", out, st, want)
	}
}

// And the array-literal ordering the scope reverses: the elements have to
// land in the local rather than in the caller, which is the opposite of what
// a `readonly` that only freezes needs.
func TestAReadonlyArrayLiteralLandsInTheLocal(t *testing.T) {
	out, st := answersRun(t, `k() { readonly -a R=(a b); printf 'in=[%s]\n' "${R[*]}"; }
k
printf 'out=[%s]\n' "${R[*]-unset}"`)
	want := "in=[a b]\nout=[unset]\n"
	if out != want || st != 0 {
		t.Errorf("a readonly array literal = %q status %d, want %q", out, st, want)
	}
}
