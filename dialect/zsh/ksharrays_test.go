// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `ksharrays` and the brackets an unbraced name carries (#1726).
//
// Every probe here uses `a=(xx yy zz)` and subscript 1, because that is the
// smallest shape where the three readings in play give three different
// answers: `yy` is the subscript read against a zero base, `xx` is the
// parameter with the brackets gone, and `xx[1]` is the answer — the element
// at the base position followed by the three characters. A one-element array
// or a subscript of 0 cannot tell them apart.
const kshArraysArray = "a=(xx yy zz)\n"

// The measurement this is all against, zsh 5.9.2 and zsh 5.9 alike:
//
//	setopt ksharrays; a=(xx yy zz); echo "$a[1]"      xx[1]
//	                                echo "${a[1]}"    yy
func TestKshArraysStopsAnUnbracedSubscriptBeingOne(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "setopt ksharrays\n"+kshArraysArray+`echo "$a[1]"`)
	if st != 0 || strings.TrimSpace(out) != "xx[1]" {
		t.Errorf("`$a[1]` under ksharrays = %q (status %d), want xx[1]", out, st)
	}
	// Both halves have to be wrong before this passes: `yy` is the subscript
	// still being read, and `xx` is it being dropped without the brackets
	// coming back as text.
	if s := strings.TrimSpace(out); s == "yy" || s == "xx" {
		t.Errorf("`$a[1]` under ksharrays = %q, which is one of the two half-answers", s)
	}
}

// The braced spelling is untouched: the braces settle where the expansion
// ends without anything having to ask, so only the base moves.
func TestKshArraysLeavesTheBracedSubscriptAlone(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"on", "setopt ksharrays\n" + kshArraysArray + `echo "${a[1]}"`, "yy"},
		{"off", kshArraysArray + `echo "${a[1]}"`, "xx"},
		{"on, unbraced", "setopt ksharrays\n" + kshArraysArray + `echo "$a[1]"`, "xx[1]"},
		{"off, unbraced", kshArraysArray + `echo "$a[1]"`, "xx"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if st != 0 || strings.TrimSpace(out) != tc.want {
				t.Errorf("= %q (status %d), want %q", out, st, tc.want)
			}
		})
	}
}

// Every unbraced shape the grammar has, not only a name with a number in it.
// Measured on zsh 5.9.2 with `a=(xxx yy zz)` and `set -- p q r`.
func TestKshArraysReachesEveryUnbracedShape(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the whole-array subscript", `echo "$a[@]"`, "xxx[@]"},
		{"a range", `echo "$a[1,2]"`, "xxx[1,2]"},
		{"a search flag group", `echo "$a[(r)yy]"`, "xxx[(r)yy]"},
		{"a length sigil in front", `echo "$#a[1]"`, "3[1]"},
		{"a set-test sigil in front", `echo "$+a[1]"`, "1[1]"},
		{"a second bracket after it", `echo "$a[1][2]"`, "xxx[1][2]"},
		{"the positionals, joined", `set -- p q r; echo "$*[1]"`, "p q r[1]"},
		{"the positionals, one by one", `set -- p q r; echo "$@[1]"`, "p q r[1]"},
		{"a name nothing ever set", `echo "[$nope[1]]"`, "[[1]]"},
		{"a scalar, whose characters are also subscripted", `s=hello; echo "$s[1]"`, "hello[1]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), "setopt ksharrays\na=(xxx yy zz)\n"+tc.src)
			if st != 0 || strings.TrimSpace(out) != tc.want {
				t.Errorf("= %q (status %d), want %q", out, st, tc.want)
			}
		})
	}
}

// The brackets are the word's text and not a second kind of it: what is
// written between them is expanded, and an unquoted `[` is live for the glob
// stage — which is the whole point of the option for a script written for
// another shell, where `$dir[0-9]*` is a pattern and never an element.
func TestKshArraysBracketsAreTheWordsOwnText(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "xx1"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, src, want string }{
		{"an expansion between them", `b=2; echo "$a[$b]"`, "xx[2]"},
		{"a substitution between them", `echo "$a[$(echo 2)]"`, "xx[2]"},
		{"unquoted, the word is a pattern", `echo $a[1]`, "xx1"},
		{"quoted, it is not", `echo "$a[1]"`, "xx[1]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, "setopt ksharrays\n"+kshArraysArray+tc.src)
			if st != 0 || strings.TrimSpace(out) != tc.want {
				t.Errorf("= %q (status %d), want %q", out, st, tc.want)
			}
		})
	}
}

// A `case` arm and a `[[ ]]` operand read the same spans a word does, so the
// brackets are text there too — and text in a pattern is a pattern. Measured:
// under the option `$a[1]` on `a=(x y)` matches `x1`, which is the bracket
// expression the two characters make, and does not match without it.
func TestKshArraysBracketsReachAPattern(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a condition", `[[ x1 == $a[1] ]] && echo M || echo N`},
		{"a case arm", `case x1 in $a[1]) echo M;; *) echo N;; esac`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			on, st := runZsh(t, t.TempDir(), "setopt ksharrays\na=(x y)\n"+tc.src)
			if st != 0 || strings.TrimSpace(on) != "M" {
				t.Errorf("on = %q (status %d), want M — the brackets are the pattern's", on, st)
			}
			off, st := runZsh(t, t.TempDir(), "a=(x y)\n"+tc.src)
			if st != 0 || strings.TrimSpace(off) != "N" {
				t.Errorf("off = %q (status %d), want N — the subscript is `x` alone", off, st)
			}
		})
	}
}

// The answer is the one in force when the word is *expanded*, not when it was
// read, which is why it is a semantics axis and not a grammar flag. Measured
// four ways on zsh 5.9.2: a function body written under either answer takes
// the caller's, `eval` takes the answer at the eval, and a sourced file takes
// the answer at the call rather than at the source.
func TestKshArraysIsAnsweredWhenTheWordExpands(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a body read without the option and called with it",
			kshArraysArray + `f() { echo "$a[1]"; }` + "\nsetopt ksharrays\nf",
			"xx[1]",
		},
		{
			"a body read with the option and called without it",
			"setopt ksharrays\n" + kshArraysArray + `f() { echo "$a[1]"; }` + "\nunsetopt ksharrays\nf",
			"xx",
		},
		{
			"eval, with the option",
			"setopt ksharrays\n" + kshArraysArray + `eval 'echo "$a[1]"'`,
			"xx[1]",
		},
		{
			"eval, without it",
			kshArraysArray + `eval 'echo "$a[1]"'`,
			"xx",
		},
		{
			"the option moved between two calls of one body",
			kshArraysArray + `f() { echo "$a[1]"; }` + "\nf\nsetopt ksharrays\nf",
			"xx\nxx[1]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if st != 0 || strings.TrimSpace(out) != tc.want {
				t.Errorf("= %q (status %d), want %q", out, st, tc.want)
			}
		})
	}
}

// The same across a `source`, which is the route a real rc file takes: the
// file is read before the option moves and called after it.
func TestKshArraysReachesASourcedBody(t *testing.T) {
	dir := t.TempDir()
	lib := filepath.Join(dir, "lib.zsh")
	if err := os.WriteFile(lib, []byte(kshArraysArray+"f() { echo \"$a[1]\"; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, st := runZsh(t, dir, "source "+lib+"\nsetopt ksharrays\nf")
	if st != 0 || strings.TrimSpace(out) != "xx[1]" {
		t.Errorf("= %q (status %d), want xx[1]", out, st)
	}
}

// `ksharrays` is one name over five axes, so the option has to move all five.
// Measured on zsh 5.9.2 with `a=(xx yy zz)`.
func TestKshArraysMovesEveryArrayAxis(t *testing.T) {
	for _, tc := range []struct{ name, src, on, off string }{
		{"the base", `echo "${a[1]}"`, "yy", "xx"},
		{"a plain name's value", `echo "$a"`, "xx", "xx yy zz"},
		{"a plain name's length", `echo "${#a}"`, "2", "3"},
		{"a plain name's fields", `printf "<%s>" $a; echo`, "<xx>", "<xx><yy><zz>"},
		{"the brackets after an unbraced name", `echo "$a[1]"`, "xx[1]", "xx"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			on, st := runZsh(t, t.TempDir(), "setopt ksharrays\n"+kshArraysArray+tc.src)
			if st != 0 || strings.TrimSpace(on) != tc.on {
				t.Errorf("on = %q (status %d), want %q", on, st, tc.on)
			}
			off, st := runZsh(t, t.TempDir(), kshArraysArray+tc.src)
			if st != 0 || strings.TrimSpace(off) != tc.off {
				t.Errorf("off = %q (status %d), want %q", off, st, tc.off)
			}
		})
	}
}

// A keyed table has no base position, so "one element" means something else
// there: the first value in the order the table yields, and not the one keyed
// `0` that bash and ksh93 look for.
func TestKshArraysReadsAKeyedTablesFirstValue(t *testing.T) {
	const table = "typeset -A h\nh=(a 1 b 2)\n"
	on, st := runZsh(t, t.TempDir(), "setopt ksharrays\n"+table+`echo "[$h][${#h}][$h[a]]"`)
	if st != 0 || strings.TrimSpace(on) != "[1][1][1[a]]" {
		t.Errorf("on = %q (status %d), want [1][1][1[a]]", on, st)
	}
	off, st := runZsh(t, t.TempDir(), table+`echo "[$h][${#h}][$h[a]]"`)
	if st != 0 || strings.TrimSpace(off) != "[1 2][2][1]" {
		t.Errorf("off = %q (status %d), want [1 2][2][1]", off, st)
	}
}

// Which value "the first" is depends on an order, and #1758 is about that
// order rather than about this axis.
//
// The table above is built in sorted order, so it answers `1` under every
// reading there is: the first inserted, the first by key, and the first this
// shell lists are all the same element. This one is not — `z` is assigned
// first and sorts last — and it is the probe that says which order is really
// being read.
//
// Re-measured 2026-09-11: real zsh answers `1` here, because `z` is where its
// *hash* puts the first key; it is not insertion order either, since all six
// spellings of this table list `z m a` there. This shell lists by key, which
// is ksh93's order exactly, so it answers `2`. Recorded rather than
// reproduced — see docs/spec/semantics.md under KeyedTableOrder, and the
// keys() comment in interp/assoc.go.
func TestTheFirstValueIsTheFirstOfThisShellsOrder(t *testing.T) {
	const table = "typeset -A h\nh[z]=1\nh[a]=2\nh[m]=3\n"
	out, st := runZsh(t, t.TempDir(), "setopt ksharrays\n"+table+`echo "[$h]"`)
	if got := strings.TrimSpace(out); st != 0 || got != "[2]" {
		t.Errorf("= %q (status %d), want [2] — the value of the first key, `a`", got, st)
	}
	// And the order itself, which is the same however the table was built:
	// the three assignments in a different order list the same way.
	const other = "typeset -A h\nh[m]=3\nh[z]=1\nh[a]=2\n"
	out, st = runZsh(t, t.TempDir(), other+`echo "[${(k)h}][${(v)h}]"`)
	if got := strings.TrimSpace(out); st != 0 || got != "[a m z][2 3 1]" {
		t.Errorf("= %q (status %d), want [a m z][2 3 1]", got, st)
	}
}

// `emulate sh` and `emulate ksh` are what turn the option on in practice, so
// the whole of this has to be checked through them rather than only through
// the option's own name — a wrong answer here is a wrong answer for two
// emulation modes at once, silently.
func TestEmulationsCarryTheWholeOption(t *testing.T) {
	for _, mode := range []string{"sh", "ksh"} {
		t.Run(mode, func(t *testing.T) {
			src := "emulate " + mode + "\n" + kshArraysArray +
				`echo "[$a[1]][${a[1]}][$a][${#a}]"`
			out, st := runZsh(t, t.TempDir(), src)
			if st != 0 || strings.TrimSpace(out) != "[xx[1]][yy][xx][2]" {
				t.Errorf("= %q (status %d), want [xx[1]][yy][xx][2]", out, st)
			}
		})
	}
	// And `emulate zsh` puts every one of them back.
	out, st := runZsh(t, t.TempDir(), "emulate sh\nemulate zsh\n"+kshArraysArray+
		`echo "[$a[1]][${a[1]}][$a][${#a}]"`)
	if st != 0 || strings.TrimSpace(out) != "[xx][xx][xx yy zz][3]" {
		t.Errorf("emulate zsh = %q (status %d), want [xx][xx][xx yy zz][3]", out, st)
	}
}

// An assignment's subscript is not this: it is still read, and against the
// base the option moved. Measured — the option changes how a *word* reads,
// not how a name is written to.
func TestKshArraysLeavesAnAssignmentSubscriptAlone(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "setopt ksharrays\n"+kshArraysArray+
		"a[1]=Q\n"+`echo "${a[@]}"`)
	if st != 0 || strings.TrimSpace(out) != "xx Q zz" {
		t.Errorf("= %q (status %d), want `xx Q zz`", out, st)
	}
	// And so is a subscript inside arithmetic, which has a grammar of its
	// own and never had the unbraced form to lose.
	out, st = runZsh(t, t.TempDir(), "setopt ksharrays\nb=(1 2 3)\n"+`echo $(( b[1] ))`)
	if st != 0 || strings.TrimSpace(out) != "2" {
		t.Errorf("arithmetic = %q (status %d), want 2", out, st)
	}
}
