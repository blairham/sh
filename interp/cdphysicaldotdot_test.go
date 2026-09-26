// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A `..` under a physical resolution is the parent of where the path
// *physically is*, not a cancellation of the component written before it —
// and that is one rule, unanimous, rather than anything a dialect answers.
// Measured 2026-09-26 in a directory holding `real/deep` and a `sub/fake`
// pointing at `../real`, `cd -P sub/fake/..` arrives at the directory holding
// both of them in bash 5.3.20, bash 3.2, dash, ksh93 and zsh 5.9.2 alike,
// where canceling `fake` against the `..` first would land in `sub`.
//
// It is `-P` here and no shell is named, because the letter is in all six
// columns and this is what the letter means. The one session switch that
// reaches the same code without a letter is exercised where the shell that
// spells it lives.
//
// This was wrong in every dialect until #4590: the operand was joined against
// the working directory with a lexical clean *before* the walk, so the `..`
// the walk was there to resolve had already been taken out of the path.

// dotDotTree builds the tree and answers its root, which is resolved rather
// than taken as handed over — t.TempDir sits under a link on macOS, and an
// unresolved base would make every row here disagree with itself.
func dotDotTree(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "real", "deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../real", filepath.Join(root, "sub", "fake")); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestPhysicalCdTakesADotDotOffWhereThePathIs(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			// The row that was wrong. A lexical clean makes this `sub`;
			// the physical parent of what `fake` points at is the root.
			"a .. straight after a link",
			"cd -P sub/fake/..", "",
		},
		{
			// One component deeper, where the two readings happen to
			// agree — here so the row above cannot be read as `-P` having
			// stopped working on `..` altogether.
			"a .. one component below a link",
			"cd -P sub/fake/deep/..", "real",
		},
		{
			// The same question asked as two commands, which is what a
			// person types: the `$PWD` the second `cd` starts from is the
			// logical one and the `..` still resolves physically.
			"cd -P .. out of a logically reached directory",
			"cd sub/fake && cd -P ..", "",
		},
		{
			// A `..` that cancels an ordinary component, with the link in
			// the tail — so the walk resolves what comes *after* the `..`
			// as well.
			"a link after the ..",
			"cd -P real/../sub/fake", "real",
		},
		{
			// No link anywhere, where the physical and lexical answers
			// coincide. The row that must not move.
			"a .. with no link on the way",
			"cd -P real/deep/..", "real",
		},
		{
			// An absolute operand is the same walk.
			"an absolute operand with a ..",
			"cd -P $PWD/sub/fake/..", "",
		},
		{
			// `-L` is the other letter and keeps the lexical reading,
			// which is the control that says the rows above are about
			// what `-P` asks for and not about `..` generally.
			"-L cancels lexically",
			"cd -L sub/fake/..", "sub",
		},
		{
			"-L with no letter's worth of difference",
			"cd -L real/deep/..", "real",
		},
		{
			// And neither letter is the same as `-L`, because the default
			// is `-L`.
			"no letter at all",
			"cd sub/fake/..", "sub",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := dotDotTree(t)
			out, errs := &strings.Builder{}, &strings.Builder{}
			sem := PosixSemantics()
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &Diagnostics{}, Dir: root,
				Stdout: out, Stderr: errs,
			})
			runCd(t, r, c.src+" && pwd\n")
			if errs.Len() != 0 {
				t.Fatalf("stderr = %q", errs.String())
			}
			want := root
			if c.want != "" {
				want = filepath.Join(root, c.want)
			}
			if got := strings.TrimSpace(out.String()); got != want {
				t.Errorf("%s left %q, want %q", c.src, got, want)
			}
		})
	}
}

// And an unresolvable path is still handed to the ordinary failure rather
// than answered here, which is the fallback the walk has always had: what to
// say about a directory that is not there is the failure's to say, from what
// the operating system said.
//
// **The path keeps its `..` on the way there**, which is the half this test
// used to assert the other way round. It required `cd -P nosuchdir/..` to
// *arrive*, on the reasoning that an unresolvable path falls through to the
// ordinary join — and the join cleans, so the `..` cancelled a component
// nobody had looked at and the shell ended up where it already was. Measured
// 2026-09-26, all six columns refuse that: bash 5.3, bash 3.2, zsh and ksh93
// at 1 with the kernel's reason, dash and BusyBox ash at 2 with `can't cd
// to nosuchdir/..`. It is unanimous *whatever* a column answers for
// Semantics.CdCancelsADotDot, because under `-P` every one of them walks the
// path with the `..` in place (#4627).
//
// The rows below are what the fallback is still for: a path with no `..` in
// it at all, and a symbolic-link cycle, both of which the walk gives up on
// and both of which the kernel then explains in the dialect's own words.
func TestAPhysicalCdThatCannotResolveIsTheOrdinaryFailure(t *testing.T) {
	root := dotDotTree(t)
	if err := os.Symlink("loop", filepath.Join(root, "loop")); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ name, src, reason string }{
		{"a component that is not there", "cd -P nosuchdir", "no such file or directory"},
		{"a `..` over one", "cd -P nosuchdir/..", "no such file or directory"},
		{"a link to itself", "cd -P loop", "too many levels of symbolic links"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs := &strings.Builder{}, &strings.Builder{}
			sem := PosixSemantics()
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &Diagnostics{}, Dir: root,
				Stdout: out, Stderr: errs,
			})
			runCd(t, r, c.src+" && pwd\n")
			if !strings.Contains(strings.ToLower(errs.String()), c.reason) {
				t.Errorf("%s said %q, want %q in it", c.src, errs.String(), c.reason)
			}
			if got := strings.TrimSpace(out.String()); got != "" {
				t.Errorf("%s printed %q, want nothing: it did not arrive", c.src, got)
			}
			if r.Dir != root {
				t.Errorf("%s left the shell in %q, want %q", c.src, r.Dir, root)
			}
		})
	}
}

// The ordinary `cd` a script writes asks no session switch and consults no
// axis, which is what keeps a new question off the common path: under
// CoreSemantics, where nothing is answered, a plain `cd` still moves and says
// nothing. A switch hoisted to the top of the builtin would make every `cd`
// in every test print "the shells disagree here".
func TestAnOrdinaryCdAsksNothing(t *testing.T) {
	root := dotDotTree(t)
	for _, src := range []string{
		"cd real && pwd",
		"cd real/deep && cd .. && pwd",
		"cd sub/fake && pwd",
		"cd sub/fake/.. && pwd",
		"cd -L sub/fake && pwd",
		"cd -P sub/fake && pwd",
		"pwd",
		"pwd -L",
		"pwd -P",
	} {
		t.Run(src, func(t *testing.T) {
			out, errs := &strings.Builder{}, &strings.Builder{}
			sem := CoreSemantics()
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &Diagnostics{}, Dir: root,
				Stdout: out, Stderr: errs,
			})
			runCd(t, r, src+"\n")
			if errs.Len() != 0 {
				t.Errorf("stderr = %q; an ordinary `cd` reaches no unanswered question", errs.String())
			}
			if out.Len() == 0 {
				t.Errorf("nothing on stdout; the `pwd` did not run")
			}
		})
	}
}
