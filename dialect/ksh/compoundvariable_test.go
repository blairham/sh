// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// A compound variable is a fourth kind of thing a name can be, beside the
// scalar and the two arrays, and `c=(a=1 b=2)` is how one is written. Until
// #2620 the literal was stored as an indexed array of the two *strings*
// `a=1` and `b=2`, so `${c.a}` answered empty — which is what an unset name
// answers and is therefore indistinguishable from the name never having been
// written at all.
//
// Every row here was measured on ksh93u+ 2012-08-01, 2026-09-13, under
// `env -i PATH=/usr/bin:/bin` with a scratch HOME, and the block that ended
// up in share/suite/ksh/params.tests runs byte-identical against that binary.
func TestACompoundVariableStoresMembers(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		// The row the issue is named for.
		{`c=(a=1 b=2); printf '%s\n' "${c.a}" "${c.b}"`, "1\n2\n"},
		// A `;` between two members is the same literal as a blank: the body
		// is a list of declarations, and those are what `;` separates.
		{`c=(a=1; b=2); printf '%s\n' "${c.a}" "${c.b}"`, "1\n2\n"},
		// The listing is one line for the whole tree, members sorted
		// whatever order they were written in, with a `;` after every member
		// but the last.
		{`c=(b=2 a=1); typeset -p c`, "typeset -C c=(a=1;b=2)\n"},
		// A member lists as the name it is when it is named.
		{`c=(a=1); typeset -p c.a`, "c.a=1\n"},
		// A member that is a compound of its own, which is the nesting the
		// grammar allows and the listing has to carry.
		{`c=(a=1 b=(y=2)); printf '%s\n' "${c.b.y}"`, "2\n"},
		{`c=(a=1 b=(y=2)); typeset -p c`, "typeset -C c=(a=1;b=(y=2))\n"},
		// The body may hold declaration commands, which is what makes it a
		// small program rather than a word list: the member carries its own
		// attributes and its value is folded by them.
		{`c=(typeset -i n=5+5); printf '%s\n' "${c.n}"`, "10\n"},
		{`c=(typeset -i n=5); typeset -p c.n`, "typeset -i c.n=5\n"},
		// An array-valued member keeps its own letters in the listing, and
		// its elements are reached through the member's name.
		{`c=(a=1 q=(p r)); printf '%s\n' "${c.q[1]}"`, "r\n"},
		{`c=(a=1 q=(p r)); typeset -p c`, "typeset -C c=(a=1;typeset -a q=(p r);)\n"},
		// A bare `$c` is the whole tree laid out over several lines, and it
		// is *text*: measured, `d=$c` leaves `d` a scalar holding those
		// lines and `${d.a}` empty, so this is a rendering and not a copy.
		// Compound copy-by-value is `typeset -C d=c`, which takes the name
		// rather than its value and is #2620's remaining piece.
		{`c=(a=1 b=2); d=$c; typeset -p d`, "d=$'(\\n\\ta=1\\n\\tb=2\\n)'\n"},
		{`c=(a=1 b=2); d=$c; printf '[%s]' "${d.a}"`, "[]"},
		// A quoted first word is not an assignment, so the same parentheses
		// are an array of two strings. The reading is decided by what was
		// *written*, not by what the words expand to.
		{`c=("a=1" "b=2"); printf '%s\n' "${#c[@]}"`, "2\n"},
		{`w=a=1; c=($w); printf '%s\n' "${#c[@]}"`, "1\n"},
	} {
		if out, st := kshOut(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q at 0", c.src, out, st, c.want)
		}
	}
}

// The surfaces that walk a compound rather than read one member of it.
//
// `${!c.@}` is the generic prefix listing and not a construct of its own,
// which is measured: `c=(a=1 b=2); cx=9; ${!c@}` answers `c.a c.b cx` on
// ksh93u+, so a member is an ordinary name that happens to be spelled with a
// dot. The interior node is the part a value-table walk misses — `c.b` holds
// no value of its own — and it is listed there beside the leaf under it.
func TestACompoundVariableEnumerates(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`c=(a=1 b=2); printf '[%s]' "${!c.@}"`, "[c.a][c.b]"},
		{`c=(a=1 b=(y=2)); printf '[%s]' "${!c.@}"`, "[c.a][c.b][c.b.y]"},
		{`c=(a=1 b=(y=2 z=(w=3))); printf '[%s]' "${!c.@}"`, "[c.a][c.b][c.b.y][c.b.z][c.b.z.w]"},
		{`c=(a=1 b=(y=2)); printf '[%s]' "${!c.b.@}"`, "[c.b.y]"},
		{`c=(a=1 q=(p r)); printf '[%s]' "${!c.@}"`, "[c.a][c.q]"},
		{`c=(a=1 b=2); printf '[%s]\n' "${!c.*}"`, "[c.a c.b]\n"},
		// Read back through the name the enumeration gave, which is the use
		// the operator has. `eval` and not `${!k}`: in this shell that
		// operator answers the *name*, which is the row above. Braced,
		// because `$c.a` unbraced is `$c` followed by the two characters
		// `.a` — the dot is a name character inside a `${ }` and not after
		// a bare `$`.
		{`c=(a=1 b=2); for k in "${!c.@}"; do eval "printf '%s=%s\n' \"\$k\" \"\${$k}\""; done`, "c.a=1\nc.b=2\n"},
		// `[[ -v ]]` sees a member as the name it is. It used to decline the
		// spelling: the operator's name test knew nothing of the dot, so a
		// member that read back perfectly well answered *unset*.
		{`c=(a=1); [[ -v c.a ]] && echo set`, "set\n"},
		{`c=(a=1); [[ -v c.zz ]] || echo unset`, "unset\n"},
		// `unset` of a member takes that name away and leaves the rest.
		{`c=(a=1 b=2); unset c.a; printf '[%s]' "${c.a}" "${!c.@}"`, "[][c.b]"},
		// And `unset` of the parent takes the members with it: they would
		// otherwise outlive it and read back the value the shell had just
		// been told to forget.
		{`c=(a=1 b=2); unset c; printf '[%s]' "${c.a}" "${!c.@}"`, "[]"},
	} {
		if out, st := kshOut(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q at 0", c.src, out, st, c.want)
		}
	}
}

// A compound is one kind at a time, so a value of another kind replaces it —
// and takes the members with it, which is the half a mark-only change would
// have missed. Measured 2026-09-13; the listing rows are what a branch
// carrying only the mark got wrong, answering `typeset -C c=(a=1)` for every
// one of them.
func TestAValueOfAnotherKindRetypesACompound(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`c=(a=1); c=(x y); typeset -p c`, "typeset -a c=(x y)\n"},
		{`c=(a=1); c=(x y); printf '[%s]' "${c.a}" "${!c.@}"`, "[]"},
		{`c=(a=1); c=hello; typeset -p c`, "c=hello\n"},
		{`c=(a=1); c=hello; printf '[%s]' "${c.a}"`, "[]"},
		{`c=(a=1); typeset -a c; printf '[%s]' "${c.a}"`, "[]"},
		{`c=(a=1); typeset -A c; typeset -p c`, "typeset -A c=()\n"},
		{`c=(a=1); typeset -A c; c[k]=v; typeset -p c`, "typeset -A c=([k]=v)\n"},
		// A literal replaces the whole compound rather than merging into it;
		// `+=` is the spelling that keeps what is there.
		{`c=(a=1 b=2); c=(d=3); typeset -p c`, "typeset -C c=(d=3)\n"},
		{`c=(a=1); c+=(b=2); typeset -p c`, "typeset -C c=(a=1;b=2)\n"},
	} {
		if out, st := kshOut(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q at 0", c.src, out, st, c.want)
		}
	}
}

// The kind can be declared without a literal, which is what `-C` is for. The
// word `compound` is that same declaration under an alias, and it is graded
// in share/suite/ksh/params.tests rather than here: the alias is installed by
// the runner and this harness parses before one exists. An empty compound lists
// with its parentheses — `typeset -C c=()` — where an empty indexed array
// lists without them, so the letter is visible on a name holding nothing.
func TestTheCompoundLetterDeclaresTheKind(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`typeset -C c; typeset -p c`, "typeset -C c=()\n"},
		{`typeset -C c; c.a=1; typeset -p c`, "typeset -C c=(a=1)\n"},
		{`typeset -C c=(a=1 b=2); typeset -p c`, "typeset -C c=(a=1;b=2)\n"},
		// The letter takes whatever the name held, rather than carrying the
		// old value in as a member or a first element.
		{`c=1; typeset -C c; typeset -p c`, "typeset -C c=()\n"},
		// An empty compound renders as an empty pair of lines, which is what
		// `$c` answers where an empty array answers nothing at all.
		{`typeset -C c; printf '[%s]\n' "$c"`, "[(\n)]\n"},
	} {
		if out, st := kshOut(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q at 0", c.src, out, st, c.want)
		}
	}
}

// A dotted name whose head is *not* a compound is two ordinary scalars, which
// is what says the dot is a name character and not a tree operator. Measured:
// `a=1; a.b=2` is accepted on ksh93u+, `typeset -p a` writes `a=1` alone, and
// `${!a.@}` answers `a.b` — so a name has children whether or not anything
// declared it a compound, and what `typeset -C` adds is how the *parent*
// reads, lists and enumerates.
func TestADottedNameNeedsNoCompound(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`a=1; a.b=2; typeset -p a`, "a=1\n"},
		{`a=1; a.b=2; printf '%s\n' "${a.b}"`, "2\n"},
		{`a=1; a.b=2; printf '[%s]' "${!a.@}"`, "[a.b]"},
		// The head keeps its own value, which is the row that tells the two
		// models apart: a tree would have replaced it.
		{`a=1; a.b=2; printf '%s\n' "$a"`, "1\n"},
	} {
		if out, st := kshOut(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q at 0", c.src, out, st, c.want)
		}
	}
}

// The construct is ksh93's alone, and the flag that carries it is the
// parser's: the other dialects read `c=(a=1 b=2)` as an array of two strings
// and must go on doing so. Asserted on the dialect rather than on an output
// because that is where a leak would be — an empty declarator set is what
// makes the compound reading unreachable, and it is one line from being
// filled in by a copy-paste.
func TestCompoundVariablesAreKshsAlone(t *testing.T) {
	if len(ksh.Dialect().CompoundVariableDeclarators) == 0 {
		t.Errorf("ksh: CompoundVariableDeclarators is empty, want the declarator words")
	}
	for _, d := range []struct {
		name string
		dia  syntax.Dialect
	}{
		{"bash", bash.Dialect()},
		{"zsh", zsh.Dialect()},
	} {
		if got := d.dia.CompoundVariableDeclarators; len(got) != 0 {
			t.Errorf("%s: CompoundVariableDeclarators = %v, want none", d.name, got)
		}
	}
}

// And the letter is a letter this shell now has, so it must be gone from the
// list of the ones it does not — the paired tables that have drifted apart
// before. A letter in both reads as accepted and then refused.
func TestTheCompoundLetterIsNoLongerRefused(t *testing.T) {
	d := ksh.Diagnostics()
	s := ksh.Semantics()
	for _, name := range []string{"typeset", "integer"} {
		if strings.ContainsRune(d.UnimplementedOptionLetters[name], 'C') {
			t.Errorf("UnimplementedOptionLetters[%s] = %q, which still claims -C is missing",
				name, d.UnimplementedOptionLetters[name])
		}
	}
	if !strings.ContainsRune(s.DeclareOptions, 'C') {
		t.Errorf("DeclareOptions = %q, want the C letter", s.DeclareOptions)
	}
	if !strings.ContainsRune(s.IntegerOptions, 'C') {
		t.Errorf("IntegerOptions = %q, want the C letter", s.IntegerOptions)
	}
}
