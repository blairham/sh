// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// `unset` of a name a *calling* function made local, and `shopt
// localvar_unset` over it. The two are one axis —
// interp.Semantics.UnsetRemovesAnEnclosingLocal — which is why the option
// could not be granted by writing `true` into a table: the default answer was
// already the option's, so a script that set it was refused for asking to
// keep what it had and a script that never mentioned it read the wrong value
// in silence (#3435).
//
// Measured 2026-09-16 against bash 5.3.20 and bash 3.2.57, `env -i
// PATH=/usr/bin:/bin LC_ALL=C` from a script file with stdin closed. The
// option itself is 5.3's: bash 3.2 answers `localvar_unset: invalid shell
// option name`, and the *default* both builds give is the one below.
func TestUnsetTakesACallersLocalAway(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The issue's own repro. Every other column in the panel answers
		// `UNSET`, `UNSET`, `GLOBAL`; here the local is taken away and the
		// global shows through from inside `g` onwards.
		{
			`v=GLOBAL
g() { unset v; echo "in g: [${v-UNSET}]"; }
f() { local v=L; g; echo "back in f: [${v-UNSET}]"; }
f
echo "global [${v-UNSET}]"`,
			"in g: [GLOBAL]\nback in f: [GLOBAL]\nglobal [GLOBAL]\n",
		},
		// At the same scope the answer is the other one, which is what makes
		// this about a *previous* scope rather than about locals.
		{
			`v=GLOBAL
f() { local v=L; unset v; echo "[${v-UNSET}]"; }
f
echo "global [${v-UNSET}]"`,
			"[UNSET]\nglobal [GLOBAL]\n",
		},
		// One binding and not every one of them: the innermost enclosing
		// call's goes and the next one out answers.
		{
			`v=GLOBAL
h() { unset v; echo "h [${v-UNSET}]"; }
g() { local v=G; h; echo "g [${v-UNSET}]"; }
f() { local v=F; g; echo "f [${v-UNSET}]"; }
f
echo "global [${v-UNSET}]"`,
			"h [F]\ng [F]\nf [F]\nglobal [GLOBAL]\n",
		},
		// The binding is gone rather than hidden: an assignment afterwards
		// reaches the global and outlives the call that used to own the name.
		{
			`v=GLOBAL
g() { unset v; v=NEW; }
f() { local v=L; g; echo "f [${v-UNSET}]"; }
f
echo "global [${v-UNSET}]"`,
			"f [NEW]\nglobal [NEW]\n",
		},
		// And the attributes of the binding that comes back come back with
		// it, which a fix that restored the value alone would miss: `3+4` is
		// stored as 7 because the global is still an integer.
		{
			`declare -i v=5
g() { unset v; v=3+4; echo "g [$v]"; }
f() { local -i v=9; g; echo "f [$v]"; }
f
echo "global [$v]"`,
			"g [7]\nf [7]\nglobal [7]\n",
		},
		// A subshell takes its own binding away and leaves the parent's call
		// holding what it declared.
		{
			`v=GLOBAL
g() { unset v; echo "g [${v-UNSET}]"; }
f() { local v=L; ( g ); echo "f [${v-UNSET}]"; }
f
echo "global [${v-UNSET}]"`,
			"g [GLOBAL]\nf [L]\nglobal [GLOBAL]\n",
		},
	} {
		out, st := runBash(t, t.TempDir(), tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%q = %q status %d, want %q status 0", tc.src, out, st, tc.want)
		}
	}
}

// `shopt -s localvar_unset` is the shell asking for the answer the rest of
// the panel gives: the name stays unset until the call that declared it
// returns. Measured on bash 5.3.20 with the option set on the same scripts.
func TestLocalvarUnsetKeepsTheBindingUntilTheCallReturns(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{
			`shopt -s localvar_unset
v=GLOBAL
g() { unset v; echo "in g: [${v-UNSET}]"; }
f() { local v=L; g; echo "back in f: [${v-UNSET}]"; }
f
echo "global [${v-UNSET}]"`,
			"in g: [UNSET]\nback in f: [UNSET]\nglobal [GLOBAL]\n",
		},
		// The binding is still there to be written, which is what says it was
		// kept rather than merely hidden: `v=NEW` inside `g` reaches the
		// caller's local and goes away with the caller.
		{
			`shopt -s localvar_unset
v=GLOBAL
g() { unset v; v=NEW; echo "g [${v-UNSET}]"; }
f() { local v=L; g; echo "f [${v-UNSET}]"; }
f
echo "global [${v-UNSET}]"`,
			"g [NEW]\nf [NEW]\nglobal [GLOBAL]\n",
		},
		// A live switch rather than a door, the way every other wired name
		// here is.
		{
			`shopt -s localvar_unset
shopt -u localvar_unset
v=GLOBAL
g() { unset v; }
f() { local v=L; g; echo "f [${v-UNSET}]"; }
f`,
			"f [GLOBAL]\n",
		},
		// A subshell keeps its own copy of the option.
		{
			`v=GLOBAL
g() { unset v; }
f() { local v=L; g; echo "[${v-UNSET}]"; }
( shopt -s localvar_unset; f )
f`,
			"[UNSET]\n[GLOBAL]\n",
		},
	} {
		out, st := runBash(t, t.TempDir(), tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%q = %q status %d, want %q status 0", tc.src, out, st, tc.want)
		}
	}
}

// The option reads back through every face of the builtin, which is what a
// harness sources when it captures a shell with `shopt -p`. It used to sit in
// the refused table, so `shopt -s localvar_unset` was `not implemented` at 1
// with the state stuck off.
func TestLocalvarUnsetReadsBack(t *testing.T) {
	for _, tc := range []struct {
		src    string
		want   string
		status int
	}{
		{`shopt localvar_unset`, "localvar_unset      \toff\n", 1},
		{`shopt -s localvar_unset; shopt localvar_unset`, "localvar_unset      \ton\n", 0},
		{`shopt -p localvar_unset`, "shopt -u localvar_unset\n", 1},
		{`shopt -s localvar_unset; shopt -p localvar_unset`, "shopt -s localvar_unset\n", 0},
		{`shopt -q localvar_unset`, "", 1},
		{`shopt -s localvar_unset; shopt -q localvar_unset`, "", 0},
		{`shopt -s localvar_unset; shopt -u localvar_unset; shopt -q localvar_unset`, "", 1},
		// Not a `set -o` name, which is what bash answers for it.
		{`shopt -o localvar_unset`, "bash: line 1: shopt: localvar_unset: invalid option name\n", 1},
	} {
		out, st := runBash(t, t.TempDir(), tc.src)
		if out != tc.want || st != tc.status {
			t.Errorf("%s = %q status %d, want %q status %d",
				tc.src, out, st, tc.want, tc.status)
		}
	}
}
