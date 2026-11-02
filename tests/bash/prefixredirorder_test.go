// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// This shell works through a command's assignment prefix **before** it opens
// the command's redirections, so a substitution in a value still runs when one
// fails — and it does so for every command, not only where the prefix would
// persist. Measured 2026-09-18 on bash 5.3.20 from a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` with a scratch HOME (#3449).
func TestThePrefixIsExpandedBeforeTheRedirectionsOpen(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, src := range []string{
		`f() { :; }; w=$(echo S >&2) f > /nope/x`,
		`w=$(echo S >&2) : > /nope/x`,
		`w=$(echo S >&2) true > /nope/x`,
		`w=$(echo S >&2) /usr/bin/true > /nope/x`,
	} {
		out, st := runBash(t, dir, src)
		if !strings.HasPrefix(out, "S\n") || st == 0 {
			t.Errorf("%s = %q (status %d), want the value's side effect first", src, out, st)
		}
	}
}

// And the walk is in written order: an earlier word's value is expanded before
// a later word is refused, and the refused word's own value is never expanded.
func TestARefusedPrefixWordIsRefusedWhereItStands(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out, _ := runBash(t, dir, `f() { :; }; w=$(echo S1 >&2) a[1]=v f`)
	side, refusal := strings.Index(out, "S1"), strings.Index(out, "a[1]")
	if side < 0 || refusal < 0 || side > refusal {
		t.Errorf("got %q, want S1 before the identifier complaint", out)
	}
	if out, _ := runBash(t, dir, `f() { :; }; a[1]=$(echo SIDE >&2) f`); strings.Contains(out, "SIDE") {
		t.Errorf("got %q, want the refused word's own value left unexpanded", out)
	}
	// And under a trace the complaint lands *between* the lines of the
	// entries around it, which is the same order read a third way.
	out, _ = runBash(t, dir, "f() { :; }\nset -x\nw=1 a[1]=v q=1 f")
	want := []string{"+ w=1", "a[1]", "+ q=1", "+ f"}
	at := -1
	for _, w := range want {
		i := strings.Index(out, w)
		if i <= at {
			t.Fatalf("got %q, want %v in that order", out, want)
		}
		at = i
	}
}

// And a subscripted word written ahead of a frozen one is refused first, which
// is the same walk met from the frozen entry's side: each word is refused where
// the walk reaches it, and the command still runs at 0. Measured 2026-11-02
// against GNU bash 5.3.20 from `-c` under `env -i PATH=/usr/bin:/bin` with a
// scratch HOME. This wrote the frozen entry's refusal, and every value behind
// it, ahead of the subscripted words' (#7095). The last row is the control,
// with the frozen entry written first.
func TestASubscriptedWordAheadOfAFrozenOneIsRefusedFirst(t *testing.T) {
	t.Parallel()
	const (
		ident = "bash: line 1: `a[1]': not a valid identifier\n"
		ro    = "bash: line 1: r: readonly variable\n"
	)
	for _, c := range []struct{ name, src, want string }{
		{"a special builtin", `readonly r=1; a[1]=x r=2 :; echo st=$?`, ident + ro + "st=0\n"},
		{"an external", `readonly r=1; a[1]=x r=2 /usr/bin/true; echo st=$?`, ident + ro + "st=0\n"},
		{
			"a function", `readonly r=1; f() { echo F; }; a[1]=x r=2 f; echo st=$?`,
			ident + ro + "F\nst=0\n",
		},
		{
			"values and words on both sides",
			`readonly r=1; w=$(echo W >&2) a[1]=x r=2 b[2]=y q=$(echo Q >&2) :`,
			"W\n" + ident + ro + "bash: line 1: `b[2]': not a valid identifier\nQ\n",
		},
		{"control: the frozen entry first", `readonly r=1; r=2 a[1]=x :; echo st=$?`, ro + ident + "st=0\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if out, _ := runBash(t, t.TempDir(), c.src); out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
