// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A bare `~` is the home the shell holds *now*, and it does not move when
// something starts a child.
//
// The second half is the whole point of the test. One build of one column
// answers a bare `~` from a copy of `HOME` that its own assignments do not
// reach, and refreshes that copy as a side effect of building an environment
// for a child — so `~` there is one answer before an external command on the
// line and a different one after it. Measured 2026-09-19, `env -i
// PATH=/usr/bin:/bin LC_ALL=C HOME=/orig`, `HOME=/h; echo ~`:
//
//	bash 5.3.20          /orig     and /h once a child has been built
//	bash as sh           /orig     the same binary, the same answer
//	bash 3.2.57          /h
//	zsh 5.9.2            /h
//	ksh93u+              /h
//	dash 0.5.12          /h
//	BusyBox ash 1.37.0   /h
//
// Six of the seven read the variable, and the seventh disagrees with its own
// older build. That seventh is modeled now — as
// [Semantics.TildeReadsACachedHome], answered Yes in one dialect and No in
// every other — and what is pinned here is the **No** answer: that the six
// columns' reading is the variable and that it is *stable*, a guard against
// the divergent reading arriving by accident or by a flipped default, in
// which `HOME=/x; cd ~` would go to the old home for as long as a script runs
// nothing but builtins.
//
// The two are one pair and neither half stands alone. The Yes answer has a
// test of its own beside the dialect that takes it —
// dialect/bash/cachedhome_test.go — and it is the *twelve* rows there that
// say what the cache is, because a cache a child's environment refreshes and
// a home frozen at startup agree on everything this file asks. See
// docs/spec/grammar/expansion.md, #3484 and #4039.
func TestABareTildeReadsTheCurrentHomeBeforeAndAfterAChild(t *testing.T) {
	dir := t.TempDir()
	// Three reads, and the order is what makes them discriminate. One before
	// the assignment fixes what the shell started with, one after it, and one
	// after a child has been built. The three readings give three different
	// triples, so no two of them can be confused:
	//
	//	the variable, read every time   dir, /h, /h     what this shell does
	//	frozen at startup               dir, dir, dir
	//	a cache a child refreshes       dir, dir, /h
	//
	// A probe with only the last two lines cannot tell any of them apart,
	// which is how the first draft of this test passed a runner mutated to
	// freeze the home it first read.
	// The axis is read off the core vector rather than set here, so that a
	// default flipped to the one column's reading fails this test instead of
	// quietly changing what it measures. See Semantics.TildeReadsACachedHome.
	if a := CoreSemantics().TildeReadsACachedHome; a != No {
		t.Fatalf("the core answers TildeReadsACachedHome %v, want No", a)
	}
	out, st := run(t, "echo ~\nHOME=/h\necho ~\n/usr/bin/true\necho ~\n", func(r *Runner) {
		sem := CoreSemantics()
		r.Semantics, r.Dir = &sem, dir
		r.Vars = map[string]string{"HOME": dir, "PATH": "/usr/bin:/bin"}
	})
	if st != 0 {
		t.Fatalf("status = %d, want 0; out = %q", st, out)
	}
	want := []string{dir, "/h", "/h"}
	if got := strings.Split(strings.TrimSpace(out), "\n"); !slices.Equal(got, want) {
		t.Errorf("the three tildes read %q, want %q", got, want)
	}
}

// TestEveryTildeRoadReadsTheOneCurrentHome: a tilde is reached from an
// ordinary word, from an assignment's value, from the tilde an assignment
// adds after each unquoted colon, from an array literal's element, from a
// redirection's target and from a `cd` operand — and there is **one** home
// behind all six.
//
// The panel says so: measured 2026-09-21 and recorded as
// `expand/tilde-after-a-home-assignment-reaches-every-road`, all seven
// columns answer the assignment, the colon and `cd ~` the same way they
// answer a bare word, whichever way that is. bash 5.3 says its cached home
// three times and the other six say the current variable three times; no
// column mixes them.
//
// So this is a guard against the shape rather than against a behavior that
// is currently wrong. There are two places in this package that turn a `~`
// into a home — the word road and the assignment's colon road — and a fix
// written into one of them and not the other would produce a shell no
// column has, silently, on lines as ordinary as `PATH=~/bin:~/sbin`. The
// second helper carrying a different answer from the first is how that goes
// wrong in practice, so the test asks every road in one place.
//
// Each road is read three times, for the reason the test above it is: at
// startup, after the script assigns, and after a child has been built. A
// probe with fewer readings cannot tell the variable from a home frozen at
// startup or from a cache a child refreshes, and #4039 is the second issue
// filed here from one that could not.
func TestEveryTildeRoadReadsTheOneCurrentHome(t *testing.T) {
	first := t.TempDir()
	second := filepath.Join(t.TempDir(), "second")
	if err := os.MkdirAll(second, 0o700); err != nil {
		t.Fatal(err)
	}
	// Both homes are real directories, because one of the roads is `cd` and
	// a road that cannot complete is a road that pins nothing.
	for _, road := range []struct{ name, read string }{
		{"an ordinary word", `echo ~`},
		{"an assignment's value", `x=~; echo "$x"`},
		{"a tilde after a colon in an assignment", `v=a:~; echo "${v#a:}"`},
		{"an array literal's element", `a=(~); echo "${a[0]}"`},
		{"a redirection's target", `: >~/mark; echo ~`},
		{"a cd operand", `cd ~; pwd`},
	} {
		t.Run(road.name, func(t *testing.T) {
			src := road.read + "\nHOME=" + second + "\n" + road.read +
				"\n/usr/bin/true\n" + road.read + "\n"
			out, st := run(t, src, func(r *Runner) {
				sem := CoreSemantics()
				// One axis is answered, and it is not this test's subject:
				// an array literal cannot be read back without saying where
				// its subscripts start, and a core vector refuses by name.
				sem.ArrayBaseIsZero = Yes
				r.Semantics, r.Dir = &sem, first
				// The environment carries the same pair, and that is what
				// makes a freeze falsifiable here: a runner handed no
				// environment has no startup value to freeze onto, so a
				// mutation reading one would find nothing and leave the
				// tilde as written — which fails the test for the wrong
				// reason and would pass a mutation that froze correctly.
				r.Env = []string{"HOME=" + first, "PATH=/usr/bin:/bin"}
				r.Vars = map[string]string{"HOME": first, "PATH": "/usr/bin:/bin"}
			})
			if st != 0 {
				t.Fatalf("status = %d, want 0; out = %q", st, out)
			}
			want := []string{first, second, second}
			if got := strings.Split(strings.TrimSpace(out), "\n"); !slices.Equal(got, want) {
				t.Errorf("the three reads gave %q, want %q", got, want)
			}
		})
	}
}
