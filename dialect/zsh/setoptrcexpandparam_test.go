// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `RC_EXPAND_PARAM` makes a parameter expansion that writes no `^` of its own
// distribute over the word it stands in: `a=(1 2); x${a}y` is the two words
// `x1y x2y` rather than the one word `x1 2y`.
//
// It was accepted and then ignored until #4549 — the name went into the
// recorded store and the word builder never read it — and what that cost is
// the **argv word count**: a command received one argument where it should
// have received two. So every row below is run in both states and the counted
// prefix is part of the expectation, because a row that compared only the
// joined text would pass against a shell that produced one word.
//
// The distribution itself was already built: `${^a}` has worked since #1517,
// and this option is the default that spelling overrides rather than a second
// implementation of it. See TestRcExpandParamAndTheCaretFlagAreOneMechanism.
//
// Measured 2026-09-25 against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m` says *not a Go executable* —
// run with `-f`, with `setopt rcexpandparam` and `unsetopt rcexpandparam`
// around each word.
const rcWords = `f(){ printf "%d:" "$#"; printf "[%s]" "$@"; printf '\n'; }` + "\n"

func TestRcExpandParamDistributesTheWordOverTheElements(t *testing.T) {
	for _, c := range []struct{ name, src, on, off string }{
		// The issue's own reduction.
		{"the reduction", `a=(1 2); f x${a}y`, "2:[x1y][x2y]\n", "2:[x1][2y]\n"},
		{"the short spelling", `a=(1 2); f x$a`, "2:[x1][x2]\n", "2:[x1][2]\n"},
		{"an explicit [@]", `a=(1 2); f x${a[@]}y`, "2:[x1y][x2y]\n", "2:[x1][2y]\n"},
		{"an explicit [*]", `a=(1 2); f x${a[*]}y`, "2:[x1y][x2y]\n", "2:[x1][2y]\n"},
		{"a prefix alone", `a=(1 2); f x${a}`, "2:[x1][x2]\n", "2:[x1][2]\n"},
		{"a suffix alone", `a=(1 2); f ${a}y`, "2:[1y][2y]\n", "2:[1][2y]\n"},
		{"an operator runs first", `a=(p1 p2); f x${a#p}y`, "2:[x1y][x2y]\n", "2:[x1][2y]\n"},
		{
			"two spans are a cross product",
			`a=(1 2); f ${a}z${a}`,
			"4:[1z1][1z2][2z1][2z2]\n", "3:[1][2z1][2]\n",
		},
		{
			"three spans, the last varying fastest",
			`a=(1 2); f x${a}y${a}z${a}`,
			"8:[x1y1z1][x1y1z2][x1y2z1][x1y2z2][x2y1z1][x2y1z2][x2y2z1][x2y2z2]\n",
			"4:[x1][2y1][2z1][2]\n",
		},
		// An associative array's values are a list like any other.
		{
			"an associative array",
			"typeset -A h=(k1 v1)\nf x${h}y\n",
			"1:[xv1y]\n", "1:[xv1y]\n",
		},
		// An empty list is the one place the distributive rule parts company
		// with the lay-in rule on nothing: the word is produced once per
		// element and there are no elements.
		{"an empty array takes the word", `a=(); f x${a}y`, "0:[]\n", "1:[xy]\n"},
		{"an empty array with no text", `a=(); f ${a}`, "0:[]\n", "0:[]\n"},
		{"no positional parameters", `f x${@}y`, "0:[]\n", "1:[xy]\n"},
		// Quoting is not a third question. `"x${a}y"` is one word in both
		// states because the quotes joined the fields before the rule ran,
		// and `"x${a[@]}y"` moves because `[@]` keeps its fields through
		// them. A guard on quoting gets the first right and the second wrong.
		{"a quoted join is one word either way", `a=(1 2); f "x${a}y"`, "1:[x1 2y]\n", "1:[x1 2y]\n"},
		{"a quoted [@] keeps its fields", `a=(1 2); f "x${a[@]}y"`, "2:[x1y][x2y]\n", "2:[x1][2y]\n"},
		{"a quoted [*] is the join again", `a=(1 2); f "x${a[*]}y"`, "1:[x1 2y]\n", "1:[x1 2y]\n"},
		{"the split flag, through quotes", `v="p q"; f "x${=v}y"`, "2:[xpy][xqy]\n", "2:[xp][qy]\n"},
		// A span that came to one field has nothing to spread, so these are
		// the no-ops — and they are here as controls rather than as claims.
		{"a scalar is a no-op", `s=scalar; f x${s}y`, "1:[xscalary]\n", "1:[xscalary]\n"},
		{"one element is a no-op", `a=(1 2); f x${a[1]}y`, "1:[x1y]\n", "1:[x1y]\n"},
		{"a count is a no-op", `a=(1 2); f x${#a}y`, "1:[x2y]\n", "1:[x2y]\n"},
		{"a join flag ran first", `a=(1 2); f x${(j:-:)a}y`, "1:[x1-2y]\n", "1:[x1-2y]\n"},
		{"arithmetic is a no-op", `f x$((1+1))y`, "1:[x2y]\n", "1:[x2y]\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, state := range []struct{ name, setopt, want string }{
				{"on", "setopt rcexpandparam\n", c.on},
				{"off", "unsetopt rcexpandparam\n", c.off},
			} {
				t.Run(state.name, func(t *testing.T) {
					out, st := answersRun(t, rcWords+state.setopt+c.src+"\n")
					if out != state.want || st != 0 {
						t.Errorf("got %q status %d, want %q at 0", out, st, state.want)
					}
				})
			}
		})
	}
}

// **The rule is keyed on the parameter expansion, not on the fields in the
// word**, and this is the pair that says so rather than a grid.
//
// A grid of array references cannot tell those two readings apart, because
// every row in such a grid *is* a parameter expansion — the hazard #4485 hit
// in this same code. So both rows here are the same command producing the
// same two fields in the same word shape, and the only thing that varies is
// whether a `${…}` wraps it. Only the wrapped one moves.
//
// Measured on zsh 5.9.2, 2026-09-25, `-f`.
func TestRcExpandParamReachesAParameterExpansionAndNotAnyFields(t *testing.T) {
	for _, c := range []struct{ name, src, on, off string }{
		{
			"a bare command substitution",
			`f x$(printf 'p q')y`,
			"2:[xp][qy]\n", "2:[xp][qy]\n",
		},
		{
			"the same command inside a parameter expansion",
			"f x${(f)\"$(printf 'p\\nq')\"}y",
			"2:[xpy][xqy]\n", "2:[xp][qy]\n",
		},
		{
			"a backquoted substitution",
			"f x`printf 'p q'`y",
			"2:[xp][qy]\n", "2:[xp][qy]\n",
		},
		{
			"two command substitutions in one word",
			`f x$(printf 'p q')y$(printf 'r s')z`,
			"3:[xp][qyr][sz]\n", "3:[xp][qyr][sz]\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, state := range []struct{ name, setopt, want string }{
				{"on", "setopt rcexpandparam\n", c.on},
				{"off", "unsetopt rcexpandparam\n", c.off},
			} {
				t.Run(state.name, func(t *testing.T) {
					out, st := answersRun(t, rcWords+state.setopt+c.src+"\n")
					if out != state.want || st != 0 {
						t.Errorf("got %q status %d, want %q at 0", out, st, state.want)
					}
				})
			}
		})
	}
}

// The option and `${^spec}` are **one mechanism read parity-first**, not two
// switches combined: the `^` characters a spec writes decide on their own,
// and only a spec with none consults the option.
//
// The two cells that settle it are the diagonal ones — `${^a}` with the
// option **off**, which distributes, and `${^^a}` with it **on**, which does
// not. An AND would lose the first and an OR would lose the second.
//
// `x${^^a}z${a}` is the sharpest of them, because it holds both readings in
// one word: with the option on, the doubled caret lays its own span in while
// the plain span beside it distributes, which no per-word combination of the
// two switches can produce.
//
// Measured on zsh 5.9.2, 2026-09-25, `-f`.
func TestRcExpandParamAndTheCaretFlagAreOneMechanism(t *testing.T) {
	for _, c := range []struct{ name, src, on, off string }{
		{"one caret", `a=(1 2); f x${^a}y`, "2:[x1y][x2y]\n", "2:[x1y][x2y]\n"},
		{"two carets", `a=(1 2); f x${^^a}y`, "2:[x1][2y]\n", "2:[x1][2y]\n"},
		{"three carets", `a=(1 2); f x${^^^a}y`, "2:[x1y][x2y]\n", "2:[x1y][x2y]\n"},
		{
			"a doubled caret beside a plain span",
			`a=(1 2); f x${^^a}z${a}`,
			"3:[x1][2z1][2z2]\n", "3:[x1][2z1][2]\n",
		},
		{
			"a plain span beside a doubled caret",
			`a=(1 2); f x${a}z${^^a}`,
			"3:[x1z1][x2z1][2]\n", "3:[x1][2z1][2]\n",
		},
		{
			"a single caret beside a plain span",
			`a=(1 2); f x${^a}z${a}`,
			"4:[x1z1][x1z2][x2z1][x2z2]\n", "3:[x1z1][x2z1][2]\n",
		},
		{
			"a quoted doubled caret still lays in",
			`a=(1 2); f "x${^^a[@]}y"`,
			"2:[x1][2y]\n", "2:[x1][2y]\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, state := range []struct{ name, setopt, want string }{
				{"on", "setopt rcexpandparam\n", c.on},
				{"off", "unsetopt rcexpandparam\n", c.off},
			} {
				t.Run(state.name, func(t *testing.T) {
					out, st := answersRun(t, rcWords+state.setopt+c.src+"\n")
					if out != state.want || st != 0 {
						t.Errorf("got %q status %d, want %q at 0", out, st, state.want)
					}
				})
			}
		})
	}
}

// The state is the semantics vector's rather than a bit in the recorded
// store, which is what makes a subshell's change stay in the subshell.
//
// Measured on zsh 5.9.2, 2026-09-25: the word inside the subshell is two and
// the one after it is one.
func TestRcExpandParamIsSubshellLocal(t *testing.T) {
	const src = rcWords +
		"a=(1 2)\n(setopt rcexpandparam; f x${a}y)\nf x${a}y\n"
	const want = "2:[x1y][x2y]\n2:[x1][2y]\n"
	out, st := answersRun(t, src)
	if out != want || st != 0 {
		t.Errorf("got %q status %d, want %q at 0", out, st, want)
	}
}

// The option still reports itself on every surface it shows on, which is the
// half that already worked and must not be traded away: moving the state onto
// the axis would be no gain if `[[ -o … ]]` and the listing then described a
// shell that no longer exists.
//
// Measured on zsh 5.9.2, 2026-09-25, `-f` throughout. `-P` is the option
// letter, which the table has carried all along.
func TestRcExpandParamReportsItsState(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"off by default", "[[ -o rcexpandparam ]] && print on || print off\n", "off\n"},
		{
			"on after setopt",
			"setopt rcexpandparam\n[[ -o rcexpandparam ]] && print on || print off\n",
			"on\n",
		},
		{
			"off again after unsetopt",
			"setopt rcexpandparam\nunsetopt rcexpandparam\n" +
				"[[ -o rcexpandparam ]] && print on || print off\n",
			"off\n",
		},
		{
			"the underscored spelling is the same name",
			"setopt rc_expand_param\n[[ -o rcexpandparam ]] && print on || print off\n",
			"on\n",
		},
		{
			"the negated spelling",
			"setopt rcexpandparam\nsetopt norcexpandparam\n" +
				"[[ -o rcexpandparam ]] && print on || print off\n",
			"off\n",
		},
		{
			"the option letter",
			"set -P\n[[ -o rcexpandparam ]] && print on || print off\n",
			"on\n",
		},
		{
			"the listing names it once set",
			"setopt rcexpandparam\nsetopt | grep '^rcexpandparam$'\n",
			"rcexpandparam\n",
		},
		{
			"$options carries it",
			"setopt rcexpandparam\nprint ${options[rcexpandparam]}\n",
			"on\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := answersRun(t, c.src)
			if out != c.want || st != 0 {
				t.Errorf("got %q status %d, want %q at 0", out, st, c.want)
			}
		})
	}
}
