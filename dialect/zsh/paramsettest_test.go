// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `${+name}` in this dialect: the is-it-set count, which one shell in the
// panel has and the other four call a bad substitution.
//
// The substrate's tests name the grammar flag; this one names the shell,
// because what "set" means is this dialect's own answer — an array, an
// associative array's key, an element out of range and a function in
// `$functions` are all it, and none of them is a question the core grammar
// can ask on its own. Measured 2026-09-06 on zsh 5.9.2.
func TestTheSetTestFlagIsThisDialects(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{`v=1; printf "[%s]" ${+v}`, `[1]`},
		{`printf "[%s]" ${+NOPE}`, `[0]`},
		{`v=1; unset v; printf "[%s]" ${+v}`, `[0]`},
		// Set-ness is not emptiness, and this is the whole reason the
		// construct is not `${v:+1}`.
		{`v=; printf "[%s]" ${+v} "${v:+X}" "${v+X}"`, `[1][][X]`},
		// A name, a positional, an array, an associative array.
		{`a=(x y z); printf "[%s]" ${+a}`, `[1]`},
		{`typeset -A m; m[k]=v; printf "[%s]" ${+m}`, `[1]`},
		{`set -- A B; printf "[%s]" ${+0} ${+1} ${+2} ${+3}`, `[1][1][1][0]`},
		// An element rather than the name: present, absent, and a key.
		{`a=(x y z); printf "[%s]" ${+a[2]} ${+a[9]}`, `[1][0]`},
		{`typeset -A m; m[k]=v; printf "[%s]" ${+m[k]} ${+m[nope]}`, `[1][0]`},
		// The spelling forty-two of zinit's uses are written in, and the
		// reason #1060's work is unreachable without this.
		{`f() { :; }; printf "[%s]" ${+functions[f]} ${+functions[nope]}`, `[1][0]`},
		{`alias xx=ls; printf "[%s]" ${+aliases[xx]} ${+aliases[qq]}`, `[1][0]`},
		{`printf "[%s]" ${+options[xtrace]} ${+options[nosuchopt]}`, `[1][0]`},
		// One field, whatever the parameter holds: an array's count does
		// not reach it and neither does its join.
		{`a=(x y z); set -- ${+a}; printf "[%s]" "$#" "$1"`, `[1][1]`},
		// The count is a number, so arithmetic reads it.
		{`v=2; printf "[%s]" $(( ${+v} + 5 )) $(( ${+nope} + 5 ))`, `[6][5]`},
		// The tilde run stands in front of it and the flag group in front
		// of both; `(P)` names a further parameter and the count is asked
		// of *that* one, while a value transformation has nothing to
		// transform.
		{`v=1; printf "[%s]" ${~+v}`, `[1]`},
		{`v=1; printf "[%s]" ${=+v} ${~=+v} ${=~+v}`, `[1][1][1]`},
		{`v=abc; printf "[%s]" ${(U)+v}`, `[1]`},
		{`w=1; v=w; printf "[%s]" ${(P)+v}`, `[1]`},
		{`v=nosuchvar; printf "[%s]" ${(P)+v}`, `[0]`},
		// With an operator written the `+` has no effect at all — it is not
		// refused, and the expansion is exactly what it would have been.
		{`v=abc; printf "[%s]" ${+v#a} ${+v:-x} ${+v+y}`, `[bc][abc][y]`},
		{`printf "[%s]" ${+nope:-D} "${+nope#a}"`, `[D][]`},
		{`printf "[%s]" ${+v=W} "$v"`, `[W][W]`},
	} {
		out, st := runZsh(t, dir, tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// Asking does not trip nounset. That is the whole of why a script guards with
// this rather than with a conditional, and it is asserted against the same
// dialect saying `parameter not set` for the plain spelling — a version that
// answered `0` because the shell had already given up would look identical
// without the second half.
func TestTheSetTestFlagDoesNotTripNounset(t *testing.T) {
	dir := t.TempDir()
	out, st := runZsh(t, dir, `set -u; printf "[%s]" ${+NOPE} ${+0}; printf "[alive]"`)
	if out != `[0][1][alive]` || st != 0 {
		t.Errorf("under set -u = %q (status %d), want `[0][1][alive]` at 0", out, st)
	}
	// Through the flag group as well, where the check has a second home and
	// the answer has to come first there too: `(P)` resolves to a name that
	// is not set, and asking about it is still asking.
	out, st = runZsh(t, dir, `set -u; v=nosuchvar; printf "[%s]" ${(P)+v}; printf "[alive]"`)
	if out != `[0][alive]` || st != 0 {
		t.Errorf("(P) under set -u = %q (status %d), want `[0][alive]` at 0", out, st)
	}
	out, st = runZsh(t, dir, `set -u; printf "[%s]" ${NOPE}; printf "[alive]"`)
	if st == 0 || !strings.Contains(out, "NOPE") {
		t.Errorf("the plain spelling = %q (status %d), want it fatal and naming NOPE", out, st)
	}
}

// A parameter that is not a name and not a positional is a bad substitution,
// and it is deferred to the run the way every unreadable expansion is in this
// dialect: in a branch never taken it is no error at all.
func TestTheSetTestFlagRefusesEverythingButANameAndDefersIt(t *testing.T) {
	dir := t.TempDir()
	for _, src := range []string{
		`printf "[%s]" ${+}`,
		`printf "[%s]" ${+?}`,
		`printf "[%s]" ${+@}`,
		`printf "[%s]" ${+#}`,
		`v=1; printf "[%s]" ${++v}`,
		`v=1; printf "[%s]" ${+#v}`,
		`v=1; printf "[%s]" ${+~v}`,
		`v=1; printf "[%s]" ${+=v}`,
	} {
		out, st := runZsh(t, dir, src)
		if st == 0 || !strings.Contains(out, "bad substitution") {
			t.Errorf("%s = %q (status %d), want a bad substitution and a nonzero status",
				src, out, st)
		}
	}
	out, st := runZsh(t, dir, `if false; then printf "[%s]" ${+?}; fi; printf "[alive]"`)
	if out != "[alive]" || st != 0 {
		t.Errorf("in a branch never taken = %q (status %d), want `[alive]` at 0", out, st)
	}
}
