// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `CHASE_LINKS` and `CHASE_DOTS` are the two names this shell has for not
// keeping the path a directory was reached by. They were accepted and then
// ignored until #4590: `setopt chaselinks` succeeded, `[[ -o chaselinks ]]`
// and the `setopt` listing reported both back faithfully, and `cd` through a
// symbolic link went on publishing the logical path in either state.
//
// Every case below is run in **both** states for that reason — a shell that
// ignores an option fails one half of every pair, where a row that only ever
// asked it one question can pass without the option being read at all.
//
// Measured against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0) — with `-f`, 2026-09-26. The fixture is built
// outside `/tmp`, which is itself a link on macOS and would put a `/private`
// prefix into every row that has nothing to do with the question.

// chaseTree builds the tree every case here runs in and answers the path with
// no link in it, which is what the rows are written against:
//
//	t/real/deep
//	t/other
//	t/sub/other
//	t/sub/fake      -> ../real
//	t/sub/sub/fake  -> ../../real
//	t/dangling      -> nowhere
//
// The **second** link is what makes `chasedots` separable from `chaselinks`.
// With only `t/sub/fake`, a destination like `sub/fake/../sub/fake` cancels
// down to a path that is not there, and the reference falls back to asking
// the kernel where it ended up — so both options agree for a reason that is
// nothing to do with either of them. With `t/sub/sub/fake` present the
// lexical answer exists too, and the three readings finally part.
//
// t.TempDir is resolved rather than used as handed over: on this machine it
// sits under `/var`, which is a link, so an unresolved base would make every
// `-P` row disagree with itself.
func chaseTree(t *testing.T) string {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "t")
	for _, d := range []string{"real/deep", "other", "sub/other", "sub/sub"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, l := range [][2]string{
		{"../real", "sub/fake"},
		{"../../real", "sub/sub/fake"},
		{"nowhere-at-all", "dangling"},
	} {
		if err := os.Symlink(l[0], filepath.Join(root, l[1])); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// runChase runs src in the tree and answers with the root's own path taken
// out, so a row reads as the shape it is about rather than as a temp path.
func runChase(t *testing.T, root, src string) string {
	t.Helper()
	out, st := runZsh(t, root, src)
	if st != 0 {
		t.Fatalf("status %d: %q", st, out)
	}
	return strings.ReplaceAll(out, root, "T")
}

// The four combinations, which is the grid the issue is about. The two names
// are separate options, they report separately, and they compose — so all
// four states are asked rather than the two that would pass if one of them
// simply turned the other on.
func TestChaseLinksAndChaseDotsOverTheFourCombinations(t *testing.T) {
	for _, c := range []struct {
		name, src                      string
		neither, dots, links, bothOpts string
	}{
		{
			// The issue's own reduction. No `..`, so only the wider
			// option has anything to do — which is the row that says the
			// two are not the same option under two names.
			name:    "through a link",
			src:     `cd sub/fake; print -r -- "$PWD"`,
			neither: "T/sub/fake\n", dots: "T/sub/fake\n",
			links: "T/real\n", bothOpts: "T/real\n",
		},
		{
			// A `..` in the destination, so both options act — the wider
			// one because it resolves everything and the narrower one
			// because there is a `..` for it to be about.
			name:    "a .. inside the destination",
			src:     `cd sub/fake/..; print -r -- "$PWD"`,
			neither: "T/sub\n", dots: "T\n",
			links: "T\n", bothOpts: "T\n",
		},
		{
			// The same question asked as two commands rather than one
			// path, which is how a person meets it.
			name:    "cd .. out of a linked directory",
			src:     `cd sub/fake; cd ..; print -r -- "$PWD"`,
			neither: "T/sub\n", dots: "T\n",
			links: "T\n", bothOpts: "T\n",
		},
		{
			// The three-way row, and the only shape in the fixture where
			// `chasedots` and `chaselinks` could have parted and do not:
			// the narrower option resolves the **whole** destination once
			// there is a `..` in it, tail included, rather than resolving
			// the `..` and leaving the rest logical. A reading that
			// resolved only the `..` would answer `T/sub/fake` here.
			name:    "a link after the ..",
			src:     `cd sub/fake/../sub/fake; print -r -- "$PWD"`,
			neither: "T/sub/sub/fake\n", dots: "T/real\n",
			links: "T/real\n", bothOpts: "T/real\n",
		},
		{
			// A `.` is not a `..`. With the narrower option on and no
			// `..` anywhere the logical name stands, which is the control
			// for the row above — it holds the option fixed and takes the
			// `..` away.
			name:    "a dot component is not this question",
			src:     `cd sub/./fake; print -r -- "$PWD"`,
			neither: "T/sub/fake\n", dots: "T/sub/fake\n",
			links: "T/real\n", bothOpts: "T/real\n",
		},
		{
			// A leading `..` with nothing written before it to cancel,
			// which still resolves — so the narrower option is not "a
			// `..` that would cancel a component" either.
			name:    "a leading .. cancels nothing and still resolves",
			src:     `cd sub; cd ../sub/fake; print -r -- "$PWD"`,
			neither: "T/sub/fake\n", dots: "T/real\n",
			links: "T/real\n", bothOpts: "T/real\n",
		},
		{
			// And the `..` need not have a link before it: here it
			// cancels an ordinary directory and the link is in the tail.
			name:    "the .. cancels an ordinary component",
			src:     `cd real/../sub/fake; print -r -- "$PWD"`,
			neither: "T/sub/fake\n", dots: "T/real\n",
			links: "T/real\n", bothOpts: "T/real\n",
		},
		{
			// A destination with a `..` and no link at all, where the
			// physical and logical answers coincide. The row that must
			// **not** move: it is what says a resolution is happening
			// rather than a rewrite that reaches every path.
			name:    "a .. with no link on the way",
			src:     `cd real/deep/..; print -r -- "$PWD"`,
			neither: "T/real\n", dots: "T/real\n",
			links: "T/real\n", bothOpts: "T/real\n",
		},
		{
			// An absolute destination is the same question: the options
			// are about following links and not about how the path was
			// spelled.
			name:    "an absolute destination",
			src:     `cd $PWD/sub/fake; print -r -- "$PWD"`,
			neither: "T/sub/fake\n", dots: "T/sub/fake\n",
			links: "T/real\n", bothOpts: "T/real\n",
		},
		{
			// `$OLDPWD` is whatever `$PWD` held, so it follows the rows
			// above rather than answering separately — and `cd -` goes
			// back to what was recorded.
			name: "OLDPWD and cd -",
			src: `cd sub/fake; cd /; print -r -- "OLDPWD=$OLDPWD"` + "\n" +
				`cd - > /dev/null; print -r -- "PWD=$PWD"`,
			neither:  "OLDPWD=T/sub/fake\nPWD=T/sub/fake\n",
			dots:     "OLDPWD=T/sub/fake\nPWD=T/sub/fake\n",
			links:    "OLDPWD=T/real\nPWD=T/real\n",
			bothOpts: "OLDPWD=T/real\nPWD=T/real\n",
		},
		{
			// `..` above the root is the root, in every state.
			name:    "above the root",
			src:     `cd /; cd ../../..; print -r -- "PWD=$PWD"`,
			neither: "PWD=/\n", dots: "PWD=/\n",
			links: "PWD=/\n", bothOpts: "PWD=/\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, state := range []struct{ name, setopt, want string }{
				{"neither", "unsetopt chaselinks chasedots\n", c.neither},
				{"chasedots", "setopt chasedots\n", c.dots},
				{"chaselinks", "setopt chaselinks\n", c.links},
				{"both", "setopt chaselinks chasedots\n", c.bothOpts},
			} {
				t.Run(state.name, func(t *testing.T) {
					root := chaseTree(t)
					if got := runChase(t, root, state.setopt+c.src); got != state.want {
						t.Errorf("got %q, want %q", got, state.want)
					}
				})
			}
		})
	}
}

// **`-L` puts one of the two down and not the other**, which is the half of
// the grid an implementation reading "the letters were written, so the
// options are out of it" gets backwards. `-P` resolves under either option
// and under neither, so it is not where the two part.
//
// Each row below holds the *destination* fixed and moves only which option is
// on, so it is the option and not the path that decides whether the letter is
// heard. Measured on zsh 5.9.2, `-f`, 2026-09-26.
func TestTheLogicalLetterOverridesChaseLinksAndNotChaseDots(t *testing.T) {
	for _, c := range []struct{ name, setopt, src, want string }{
		{
			"chaselinks, cd -L through a link",
			"setopt chaselinks\n", `cd -L sub/fake; print -r -- "$PWD"`,
			"T/sub/fake\n",
		},
		{
			"chaselinks, cd -L with a .. in it",
			"setopt chaselinks\n", `cd -L sub/fake/../sub/fake; print -r -- "$PWD"`,
			"T/sub/sub/fake\n",
		},
		{
			// The same destination and the same letter as the row above,
			// and the answer moves — so `-L` is not simply "be logical".
			"chasedots, cd -L with a .. in it",
			"setopt chasedots\n", `cd -L sub/fake/../sub/fake; print -r -- "$PWD"`,
			"T/real\n",
		},
		{
			"both, cd -L with a .. in it",
			"setopt chaselinks chasedots\n", `cd -L sub/fake/../sub/fake; print -r -- "$PWD"`,
			"T/real\n",
		},
		{
			// And the control for that pair: the same options and the
			// same letter with no `..` in the destination, where the
			// narrower option has nothing to be about and `-L` is heard.
			"both, cd -L with no .. in it",
			"setopt chaselinks chasedots\n", `cd -L sub/fake; print -r -- "$PWD"`,
			"T/sub/fake\n",
		},
		{
			"chasedots, cd -L with no .. in it",
			"setopt chasedots\n", `cd -L sub/fake; print -r -- "$PWD"`,
			"T/sub/fake\n",
		},
		{
			"cd -P with neither option",
			"unsetopt chaselinks chasedots\n", `cd -P sub/fake; print -r -- "$PWD"`,
			"T/real\n",
		},
		{
			"cd -P with chaselinks",
			"setopt chaselinks\n", `cd -P sub/fake; print -r -- "$PWD"`,
			"T/real\n",
		},
		{
			"cd -P with chasedots and a ..",
			"setopt chasedots\n", `cd -P sub/fake/..; print -r -- "$PWD"`,
			"T\n",
		},
		{
			// This shell gives `-P` the answer wherever it appears, which
			// is Semantics.CdLastPathOptionWins and not this question —
			// here to keep the two apart.
			"cd -L -P, where -P wins in this shell",
			"setopt chaselinks\n", `cd -L -P sub/fake; print -r -- "$PWD"`,
			"T/real\n",
		},
		{
			"cd -P -L, the same either way round",
			"setopt chaselinks\n", `cd -P -L sub/fake; print -r -- "$PWD"`,
			"T/real\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := chaseTree(t)
			if got := runChase(t, root, c.setopt+c.src); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// The moment each option is read is the **command that is running**, and for
// `chaselinks` that is two commands with two moments: `cd` reads it when it
// moves and `pwd` reads it when it prints. Doing the work once in `cd` and
// letting `pwd` write `$PWD` gets the second pair backwards.
//
// Measured on zsh 5.9.2, `-f`, 2026-09-26.
func TestChaseLinksIsReadAtTheCommandThatIsRunning(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			// On at the `cd` and off by the `pwd`: `$PWD` was written
			// resolved and stays resolved. Turning the option off does
			// not put the logical name back.
			"on at the cd, off at the pwd",
			`setopt chaselinks; cd sub/fake; unsetopt chaselinks` + "\n" +
				`print -r -- "PWD=$PWD"; pwd; pwd -L; pwd -P`,
			"PWD=T/real\nT/real\nT/real\nT/real\n",
		},
		{
			// Off at the `cd` and on by the `pwd`. This is the row that
			// pins it: `$PWD` still holds the logical name — so nothing
			// rewrote it — and a bare `pwd` writes the resolved one all
			// the same, while `pwd -L` writes the logical one. The option
			// makes a bare `pwd` mean `pwd -P`, read when `pwd` runs.
			"off at the cd, on at the pwd",
			`unsetopt chaselinks; cd sub/fake; setopt chaselinks` + "\n" +
				`print -r -- "PWD=$PWD"; pwd; pwd -L; pwd -P`,
			"PWD=T/sub/fake\nT/real\nT/sub/fake\nT/real\n",
		},
		{
			// The control for the pair: with the option never on, a bare
			// `pwd` and `pwd -L` agree and only `pwd -P` resolves.
			"never on",
			`unsetopt chaselinks; cd sub/fake` + "\n" +
				`print -r -- "PWD=$PWD"; pwd; pwd -L; pwd -P`,
			"PWD=T/sub/fake\nT/sub/fake\nT/sub/fake\nT/real\n",
		},
		{
			// And `chasedots` does not reach `pwd` at all: the same four
			// lines a shell with neither option writes.
			"chasedots does not reach pwd",
			`setopt chasedots; cd sub/fake` + "\n" +
				`print -r -- "PWD=$PWD"; pwd; pwd -L; pwd -P`,
			"PWD=T/sub/fake\nT/sub/fake\nT/sub/fake\nT/real\n",
		},
		{
			// The `cd` half, over two moves. On for the first and off for
			// the second: the second is an ordinary lexical `cd ..` out
			// of a `$PWD` that is already resolved.
			"chaselinks on at the first cd, off at the second",
			`setopt chaselinks; cd sub/fake/deep; unsetopt chaselinks; cd ..` + "\n" +
				`print -r -- "$PWD"`,
			"T/real\n",
		},
		{
			// Off for the first and on for the second, so `$PWD` is
			// logical when the `cd ..` runs and the `..` resolves anyway.
			// Together with the row above the state at the *first* `cd`
			// takes both values on each side of the answer, so it cannot
			// be what decides the second.
			"chaselinks off at the first cd, on at the second",
			`unsetopt chaselinks; cd sub/fake/deep; setopt chaselinks; cd ..` + "\n" +
				`print -r -- "$PWD"`,
			"T/real\n",
		},
		{
			// The control that makes that pair mean something: with the
			// option off at both, the `cd ..` is lexical and lands
			// somewhere else entirely.
			"chaselinks off at both",
			`unsetopt chaselinks; cd sub/fake/deep; cd ..; print -r -- "$PWD"`,
			"T/sub/fake\n",
		},
		{
			// The same pair for `chasedots`, where the two moments give
			// different answers rather than the same one — the option is
			// read at the `cd` that holds the `..`.
			"chasedots on at the first cd, off at the cd ..",
			`setopt chasedots; cd sub/fake; unsetopt chasedots; cd ..` + "\n" +
				`print -r -- "$PWD"`,
			"T/sub\n",
		},
		{
			"chasedots off at the first cd, on at the cd ..",
			`unsetopt chasedots; cd sub/fake; setopt chasedots; cd ..` + "\n" +
				`print -r -- "$PWD"`,
			"T\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := chaseTree(t)
			if got := runChase(t, root, c.src); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// The reporting half already worked and must not be traded away for the
// behavior, so both are asked here — and the state is scoped the way an
// option is, which is what makes `(setopt chaselinks)` stay in the subshell
// and what lets `localoptions` put it back at a function's return.
//
// Measured on zsh 5.9.2, `-f`, 2026-09-26.
func TestChaseOptionsAreReportedAndScopedLikeOptions(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"the two report apart",
			"setopt chaselinks\n" +
				"[[ -o chaselinks ]] && print CL-on\n" +
				"[[ -o chasedots ]] || print CD-off\n" +
				`print -r -- "${options[chaselinks]} ${options[chasedots]}"` + "\n",
			"CL-on\nCD-off\non off\n",
		},
		{
			// `physical` is the compat spelling of the same entry and
			// `-w` is its letter, both already in the table; they must
			// move the behavior now that the canonical name does.
			"the compat spelling moves it",
			"setopt physical\ncd sub/fake\nprint -r -- \"$PWD\"\n",
			"T/real\n",
		},
		{
			"the option letter moves it",
			"set -w\ncd sub/fake\nprint -r -- \"$PWD\"\n",
			"T/real\n",
		},
		{
			// And `nophysical` is the canonical entry off, which is what
			// keeps the compat spelling from becoming a second switch.
			"the negated compat spelling puts it back",
			"setopt chaselinks\nunsetopt physical\ncd sub/fake\nprint -r -- \"$PWD\"\n",
			"T/sub/fake\n",
		},
		{
			"a subshell keeps its change to itself",
			"unsetopt chaselinks\n" +
				"( setopt chaselinks; cd sub/fake; print -r -- \"in=$PWD\" )\n" +
				"cd sub/fake; print -r -- \"out=$PWD\"\n",
			"in=T/real\nout=T/sub/fake\n",
		},
		{
			// `localoptions` puts it back at the return, so the move
			// after the call is logical again.
			"localoptions puts it back at the return",
			"unsetopt chaselinks\n" +
				"h() { setopt localoptions chaselinks; cd sub/fake; print -r -- \"in=$PWD\" }\n" +
				"h\ncd ..\ncd sub/fake\nprint -r -- \"out=$PWD\"\n",
			"in=T/real\nout=T/sub/fake\n",
		},
		{
			// The control for the row above: the same body without
			// `localoptions` leaves the option on.
			"and without it the change stands",
			"unsetopt chaselinks\n" +
				"h() { setopt chaselinks; cd sub/fake; print -r -- \"in=$PWD\" }\n" +
				"h\ncd ..\ncd sub/fake\nprint -r -- \"out=$PWD\"\n",
			"in=T/real\nout=T/real\n",
		},
		{
			// The same scoping for the narrower name, so neither of them
			// is a bit that outlives its scope.
			"chasedots is scoped too",
			"unsetopt chasedots\n" +
				"( setopt chasedots; cd sub/fake/..; print -r -- \"in=$PWD\" )\n" +
				"cd sub/fake/..; print -r -- \"out=$PWD\"\n",
			"in=T\nout=T/sub\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := chaseTree(t)
			if got := runChase(t, root, c.src); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// Both names are written by a bare `setopt` when they are on, which is the
// listing half of the reporting and is separate from `[[ -o … ]]`.
func TestTheSetoptListingNamesTheChaseOptions(t *testing.T) {
	root := chaseTree(t)
	out := runChase(t, root, "setopt chaselinks chasedots\nsetopt\n")
	for _, want := range []string{"chasedots\n", "chaselinks\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("the `setopt` listing is %q; an option turned on is named there, want %q", out, want)
		}
	}
}
