// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// A declaration that makes a name local takes the names **under** it with it.
//
// A compound variable's members are ordinary names spelled with a dot, so
// `typeset c=(a=1)` inside a `function` body has two jobs: `c` is displaced,
// and so is everything under `c.`. Shadowing the bare name alone wrote `c.a`
// into the **caller's** scope and took the caller's own members away first —
// silently, at status 0, which is the worst shape a defect here can have.
//
// Measured on ksh93u+ 2012-08-01, 2026-09-19, `env -i PATH=/usr/bin:/bin
// LC_ALL=C /bin/ksh x.sh` over a script file with stdin on `/dev/null`
// (#3824).
func TestALocalDeclarationTakesTheNamesUnderItWithIt(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, src, want string
	}{
		{
			// The issue's own reduction. The caller's compound comes back
			// whole and the member the body made is gone.
			name: "a compound operand does not reach the caller",
			src: `c=(z=9)
function f { typeset c=(a=1); print -r -- "in=[${c.a}][${c.z}]"; }
f
print -r -- "out=[${c.a}][${c.z}]"`,
			want: "in=[1][]\nout=[][9]\n",
		},
		{
			// The same shape with nothing to clobber. Both halves matter:
			// the member is gone *and* the name is not set at all, which is
			// the compound mark being taken back as well as the value.
			name: "a compound the body invents does not outlive the call",
			src: `function f { typeset c=(a=1); }
f
print -r -- "out=[${c.a}] set=[${c+yes}]"`,
			want: "out=[] set=[]\n",
		},
		{
			// A member written by hand rather than by the operand, which is
			// what says the rule is about the declaration and not about the
			// literal spelling.
			name: "a member written in the body is the call's",
			src: `c=(z=9)
function f { typeset c; c.a=1; }
f
print -r -- "out=[${c.a}][${c.z}]"`,
			want: "out=[][9]\n",
		},
		{
			// And the fresh binding starts **empty**: the body does not read
			// the caller's members through its own declaration.
			name: "the local namespace starts empty",
			src: `c=(z=9)
function f { typeset -C c; print -r -- "in=[${c.z}]"; }
f
print -r -- "out=[${c.z}]"`,
			want: "in=[]\nout=[9]\n",
		},
		{
			// Two members, so that a row with one is not the only evidence:
			// the restore was order-dependent before the scope kept its own
			// record of what it had shadowed, and rows with two members
			// passed while rows with one failed.
			name: "every member comes back",
			src: `c=(z=9 y=8)
function f { typeset c=(a=1); }
f
print -r -- "out=[${c.a}][${c.z}][${c.y}]"`,
			want: "out=[][9][8]\n",
		},
		{
			// Nested two deep, which says the sweep is over the whole
			// namespace and not over one level of it.
			name: "a member nested two deep comes back",
			src: `c=(z=(q=7))
function f { typeset c=(a=1); }
f
print -r -- "out=[${c.z.q}][${c.a}]"`,
			want: "out=[7][]\n",
		},
		{
			// A plain scalar with a child. The rule is about the **prefix**
			// rather than about the compound mark, and this is the row that
			// says so.
			name: "a scalar with a child",
			src: `a=1
a.b=2
function f { typeset a=5; print -r -- "in=[${a.b}]"; }
f
print -r -- "out=[${a.b}][$a]"`,
			want: "in=[]\nout=[2][1]\n",
		},
		{
			// `unset` of the local leaves the caller's compound alone.
			name: "unset inside the call reaches the local",
			src: `c=(z=9)
function f { typeset c=(a=1); unset c; print -r -- "u=[${c.z}]"; }
f
print -r -- "out=[${c.z}]"`,
			want: "u=[]\nout=[9]\n",
		},
		{
			// And the listing, which reads the store rather than the value
			// and so is a second view of the same fact.
			name: "the caller's listing is the caller's compound",
			src: `c=(z=9)
function f { typeset c=(a=1); }
f
typeset -p c`,
			want: "typeset -C c=(z=9)\n",
		},
		{
			// Nested calls, each with a declaration of its own: `f` sees its
			// own member again once `g` has returned.
			name: "each call has its own namespace",
			src: `c=(z=9)
function g { typeset c=(b=2); print -r -- "g-in=[${c.b}][${c.z}]"; }
function f { typeset c=(a=1); g; print -r -- "f-after=[${c.a}][${c.b}]"; }
f
print -r -- "out=[${c.a}][${c.b}][${c.z}]"`,
			want: "g-in=[2][]\nf-after=[1][]\nout=[][][9]\n",
		},
		{
			// A static seal composes with it: this shell scopes a `function`
			// body's declarations statically, so a callee reads past the
			// caller's local namespace to the shell's own.
			name: "a callee reads past the caller's namespace",
			src: `c=(z=9)
function g { print -r -- "g=[${c.z}][${c.a}]"; }
function f { typeset c=(a=1); g; }
f`,
			want: "g=[9][]\n",
		},
		{
			// Twice, because a scope that put back the wrong thing once may
			// still put back the right thing the second time.
			name: "called twice leaves nothing behind",
			src: `function f { typeset c=(a=1); }
f
f
print -r -- "twice=[${c.a}]"`,
			want: "twice=[]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKsh(t, dir, tc.src)
			if out != tc.want {
				t.Errorf("output = %q, want %q", out, tc.want)
			}
			if st != 0 {
				t.Errorf("status = %d, want 0", st)
			}
		})
	}
}

// The controls, which say the rule is no wider than a declaration's own scope.
// Every one of these answered correctly before the namespace was shadowed too,
// and a change that reached further than it should would move one of them.
func TestANamespaceIsOnlyShadowedWhereADeclarationStands(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, src, want string
	}{
		{
			// No declaration at all, so the member write is to the shell's
			// own name and outlives the call.
			name: "a member write with no declaration",
			src: `c=(z=9)
function f { c.a=1; }
f
print -r -- "out=[${c.a}][${c.z}]"`,
			want: "out=[1][9]\n",
		},
		{
			// `typeset` in a POSIX-style body is not local in this shell at
			// all, so nothing here may fire for it. This is the control that
			// keeps the whole change honest: it hangs off the shadow, and the
			// shadow is what that axis turns away.
			name: "a POSIX-style body declares nothing local",
			src: `c=(z=9)
f() { typeset c=(a=1); }
f
print -r -- "out=[${c.a}][${c.z}]"`,
			want: "out=[1][]\n",
		},
		{
			name: "the same for a member written by hand",
			src: `c=(z=9)
f() { typeset c; c.a=1; }
f
print -r -- "out=[${c.a}][${c.z}]"`,
			want: "out=[1][9]\n",
		},
		{
			// An array literal operand, which shares the spelling and is a
			// different construct — correct before this change and after it.
			name: "an array literal operand",
			src: `a=(9 9 9)
function f { typeset a=(1 2); }
f
print -r -- "out=[${a[@]}]"`,
			want: "out=[9 9 9]\n",
		},
		{
			name: "a scalar operand",
			src: `s=keep
function f { typeset s=new; }
f
print -r -- "out=[$s]"`,
			want: "out=[keep]\n",
		},
		{
			// A **member** named outright on a declaration, which is a
			// divergence this change does not touch and is pinned here so
			// that it cannot start moving unnoticed: ksh93u+ answers
			// `in=[5] out=[5]`, so `typeset c.z=5` writes the caller's member
			// there and is local here. Measured 2026-09-19, and the same on
			// `main` — the namespace shadow reaches a member a *body* writes
			// and leaves a member a declaration *names* exactly where it was.
			name: "a member named on a declaration — a divergence, unmoved",
			src: `c=(z=9)
function f { typeset c.z=5; print -r -- "in=[${c.z}]"; }
f
print -r -- "out=[${c.z}]"`,
			want: "in=[5]\nout=[9]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKsh(t, dir, tc.src)
			if out != tc.want {
				t.Errorf("output = %q, want %q", out, tc.want)
			}
			if st != 0 {
				t.Errorf("status = %d, want 0", st)
			}
		})
	}
}

// The gate, which is what keeps this inert in the four dialects with no
// spelling that could reach it: a shell that has never held a name with a
// member separator in it does none of the work above.
func TestANamespaceShadowIsInertWithoutMembers(t *testing.T) {
	dir := t.TempDir()
	out, st := runKsh(t, dir, `v=outer
function f { typeset v=inner; print -r -- "in=[$v]"; }
f
print -r -- "out=[$v]"`)
	if want := "in=[inner]\nout=[outer]\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
	if strings.Contains(out, ".") {
		t.Errorf("output = %q, and nothing here should have a member in it", out)
	}
}
