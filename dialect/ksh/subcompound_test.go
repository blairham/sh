// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A compound variable standing where an array *element* goes — `a[1]=(p=1
// q=2)` — which is the same parentheses holding the same two constructs they
// hold over a bare name, decided by the same first word.
//
// Every row was measured on ksh93u+ 2012-08-01, 2026-09-19, each one its own
// script file under `env -i PATH=/usr/bin:/bin LC_ALL=C /bin/ksh x.sh` with
// standard input on `/dev/null` and the output read through `sed -n l`. The
// rows are written out in docs/spec/semantics.md, "A compound variable held
// in an array element" (#2853).
func TestACompoundVariableInAnArrayElement(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		// The listing joins the members with `;` and takes no trailing
		// blank, which is what tells this element from the nested array in
		// the control below.
		{`a[1]=(p=1 q=2); typeset -p a`, "typeset -a a=([1]=(p=1;q=2))\n"},
		// The element's own read is the whole compound, the same four lines
		// `"$c"` gives over a bare name.
		{`a[1]=(p=1 q=2); printf '[%s]' "${a[1]}"`, "[(\n\tp=1\n\tq=2\n)]"},
		// A member, which had no spelling in this grammar at all: the dot
		// after a `]` was `` syntax error: `.' unexpected ``.
		{`a[1]=(p=1 q=2); printf '[%s]' "${a[1].p}" "${a[1].q}"`, "[1][2]"},
		// Several links deep, because a member may itself be a compound.
		{`a[1]=(p=1 q=(r=2)); printf '[%s]' "${a[1].q.r}"`, "[2]"},
		{`a[1]=(p=1 q=(r=2)); typeset -p a`, "typeset -a a=([1]=(p=1;q=(r=2)))\n"},
		// The member enumeration answers the names themselves, which is what
		// says the members are ordinary names spelled with the element's own
		// subscript in front rather than a store of their own.
		{`a[1]=(p=1 q=2); print -r -- ${!a[1].@}`, "a[1].p a[1].q\n"},
		// And not at the head of its word, where the fields join to what
		// stands beside them exactly as `${!c.@}`'s do.
		{`a[1]=(p=1 q=2); print -r -- x${!a[1].@}`, "xa[1].p a[1].q\n"},
		{`a[1]=(p=1 q=2); printf '[%s]' x${!a[1].@}y`, "[xa[1].p][a[1].qy]"},
		// The `*` of the pair joins the names into one field inside
		// quotes, and the `@` keeps them apart — the same two readings
		// `$@` and `$*` have.
		{`a[1]=(p=1 q=2); set -- "${!a[1].@}"; echo $#`, "2\n"},
		{`a[1]=(p=1 q=2); set -- "${!a[1].*}"; echo $#`, "1\n"},
		// A member carries its own attributes, and is listed as the name it
		// is when it is named.
		{`a[1]=(typeset -i n=5); typeset -p a`, "typeset -a a=([1]=(typeset -i n=5))\n"},
		{`a[1]=(typeset -i n=5); typeset -p 'a[1].n'`, "typeset -i a[1].n=5\n"},
		// Writing a member, through the same three pieces the read takes
		// apart — and through an *expanded* subscript, which is what says
		// the namespace is the evaluated element rather than the text.
		{`a[1]=(p=1 q=2); a[1].p=9; typeset -p a`, "typeset -a a=([1]=(p=9;q=2))\n"},
		{`i=1; a[1]=(p=1); a[$i].p=7; printf '[%s]' "${a[1].p}"`, "[7]"},
		// `+=` over a compound element adds a *member*, where the nested
		// array's `+=` adds to the list inside the element.
		{`a[1]=(p=1); a[1]+=(q=2); typeset -p a`, "typeset -a a=([1]=(p=1;q=2))\n"},
		// A table's element is the same construct reached by the other route.
		{`typeset -A m; m[k]=(p=1 q=2); typeset -p m`, "typeset -A m=([k]=(p=1;q=2))\n"},
		{`typeset -A m; m[k]=(p=1 q=2); printf '[%s]' "${m[k].p}"`, "[1]"},
		// Named on its own the namespace lists as the compound it is, which
		// is the row the whole-shell listing below is a filter over.
		{`a[1]=(p=1 q=2); typeset -p 'a[1]'`, "typeset -C a[1]=(p=1;q=2)\n"},
		// The member takes the whole of the expansion grammar with it, which
		// is why it is rewritten to the plain name it is rather than given a
		// path of its own.
		{`a[1]=(p=1); printf '[%s]' "${#a[1].p}" "${a[1].p:-D}" "${a[1].zz:-D}"`, "[1][1][D]"},
		// A second subscript reads the element's text at the base and
		// nothing anywhere else, which is the rule a string element already
		// follows.
		{`a[1]=(p=1 q=2); printf '[%s]' "${a[1][0]}" "${a[1][1]}"`, "[(\n\tp=1\n\tq=2\n)][]"},
		// The element counts as one element, like any other.
		{`a[1]=(p=1 q=2); echo ${#a[@]}`, "1\n"},
		// An **empty** pair of parentheses is the compound too, and the
		// word decides it with nothing taken out. The first two rows are
		// where that is invisible — the element's own listing and read are
		// the same under either reading, which is what a guard against the
		// compound reading here was built on — and the three after them are
		// where it is not.
		{`a=(x y z); a[1]=(); typeset -p a`, "typeset -a a=(x () z)\n"},
		{`a=(x y z); a[1]=(); printf '[%s]' "${a[1]}"`, "[(\n)]"},
		{`a=(x y z); a[1]=(); typeset -p 'a[1]'`, "typeset -C a[1]=()\n"},
		{`a[1]=(); a[1].p=3; typeset -p a`, "typeset -a a=([1]=(p=3))\n"},
		{`typeset -A m; m[k]=(); typeset -p 'm[k]'`, "typeset -C m[k]=()\n"},
	} {
		if out, st := kshOut(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q at 0", c.src, out, st, c.want)
		}
	}
}

// The members are real names in the flat namespace, so every route that stops
// the element holding the compound has to take them with it — otherwise
// `${a[1].p}` reads back a value the shell has just been told to forget.
// Five spellings, measured, and each one is a different writer.
func TestACompoundElementLosesItsMembersWithItsValue(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`a[1]=(p=1); a[1]=(x y); printf '[%s]' "${a[1].p}"`, "[]"},
		{`a[1]=(p=1); a[1]=z; printf '[%s]' "${a[1].p}"`, "[]"},
		{`a[1]=(p=1); a=(x y); printf '[%s]' "${a[1].p}"`, "[]"},
		{`a[1]=(p=1); unset a; printf '[%s]' "${a[1].p}"`, "[]"},
		{`a[1]=(p=1); unset 'a[1]'; printf '[%s]' "${a[1].p}"`, "[]"},
	} {
		if out, st := kshOut(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q at 0", c.src, out, st, c.want)
		}
	}
}

// A member naming an element that is not there brings the element into being
// as an empty compound, which is the half of the member write that has no
// literal in it at all. The rule is *set*-ness and not which table the name
// has, and the last two rows are the other side of it: the reference folds
// the old value into a member name over an element that is already there —
// `typeset -A m=([k]=v); m[k].p=9` is `typeset -A m=([k]=(p=9.=v))` — which
// is bookkeeping rather than a rule, so an occupied element is left alone.
func TestAMemberBringsAnAbsentElementIntoBeing(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`a[1].p=5; typeset -p a`, "typeset -a a=([1]=(p=5))\n"},
		{`a[1].p=5; printf '[%s]' "${a[1].p}" "${a[1]}"`, "[5][(\n\tp=5\n)]"},
		// Only the first link needs it: `a[1].q` is created by the ordinary
		// compound machinery once `a[1]` stands, as `c.q` is under `c`.
		{`a[1].q.r=5; typeset -p a`, "typeset -a a=([1]=(q=(r=5)))\n"},
		// A gap in an array that is already there, and a key a table has not
		// got — the same answer by the name's other route.
		{`a=(x y); a[5].p=9; typeset -p a`, "typeset -a a=([0]=x [1]=y [5]=(p=9))\n"},
		{`a=(x y); a[2].p=9; typeset -p a`, "typeset -a a=(x y (p=9))\n"},
		{`typeset -A m=([k]=v); m[z].p=9; typeset -p m`, "typeset -A m=([k]=v [z]=(p=9))\n"},
		{`typeset -A m; m[k].p=5; typeset -p m`, "typeset -A m=([k]=(p=5))\n"},
		// A scalar is promoted to the array it would be for `a[1]=x` too.
		{`a=1; a[1].p=9; typeset -p a`, "typeset -a a=(1 (p=9))\n"},
		// And an element that *is* there is left alone, so the member write
		// falls through to the ordinary store and the element keeps its
		// value. Both rows disagree with the reference, on purpose.
		{`a=(x y); a[1].p=9; typeset -p a`, "typeset -a a=(x y)\n"},
		{`typeset -A m=([k]=v); m[k].p=9; typeset -p m`, "typeset -A m=([k]=v)\n"},
		// An element holding an **empty** compound is there, so this is the
		// ordinary member write and not a second creation.
		{`a[1]=(); a[1].p=9; typeset -p a`, "typeset -a a=([1]=(p=9))\n"},
		// The members it created go the way every other route's do.
		{`a[1].p=5; unset a; printf '[%s]' "${a[1].p}"`, "[]"},
	} {
		if out, st := kshOut(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q at 0", c.src, out, st, c.want)
		}
	}
}

// The controls, and they are what say the gap was the **compound reading**
// rather than subscripts in general: a nested *array* under a subscript was
// byte-exact against the reference before any of this, and must stay so.
func TestANestedArrayUnderASubscriptDoesNotMove(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`b[1]=(x y); typeset -p b`, "typeset -a b=([1]=(x y) )\n"},
		{`b[1]=(x y); printf '[%s]' "${b[1]}"`, "[x]"},
		{`b[1][2]=q; typeset -p b`, "typeset -a b=([1]=([2]=q) )\n"},
	} {
		if out, st := kshOut(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q at 0", c.src, out, st, c.want)
		}
	}
}

// The `A` letter does **not** take the compound reading off the literal,
// which is the half that separates it from `a` — and the compound it admits
// is stored as the element at the key `0` rather than as the name's own.
func TestTheTableLetterDoesNotTakeTheCompoundReading(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`typeset -A c=(a=1 b=2); typeset -p c`, "typeset -A c=([0]=(a=1;b=2))\n"},
		{`typeset -A c=(a=1 b=2); printf '[%s]' "${c[0].a}" "${c[0].b}"`, "[1][2]"},
		{`typeset -A c=(a=1; b=2); typeset -p c`, "typeset -A c=([0]=(a=1;b=2))\n"},
		{`typeset -A c=(typeset -i n=5); typeset -p c`, "typeset -A c=([0]=(typeset -i n=5))\n"},
		{`typeset -A c=(a=1 b=(z=2)); typeset -p c`, "typeset -A c=([0]=(a=1;b=(z=2)))\n"},
		// The `a` letter is the one that really takes the reading away.
		{`typeset -a c=(a=1 b=2); printf '[%s]' "${#c[@]}" "${c[0]}"`, "[2][a=1]"},
		// It is the **name's** table-ness and not the command's letters that
		// puts the value at a key, which is what this pair says.
		{`typeset -A c; c=(a=1 b=2); typeset -p c`, "typeset -A c=([0]=(a=1;b=2))\n"},
		{`typeset -A c=([k]=v); c=(a=1); typeset -p c`, "typeset -A c=([0]=(a=1))\n"},
		// And the shapes that are not this. An empty pair under the letter is
		// the empty table; an empty body with no letter replaces the table
		// with a compound; and `+=` with an empty body adds nothing.
		{`typeset -A c=(); typeset -p c`, "typeset -A c=()\n"},
		{`typeset -A c=([k]=v); c=(); typeset -p c`, "typeset -C c=()\n"},
		{`typeset -A c=([k]=v); c+=(); typeset -p c`, "typeset -A c=([k]=v)\n"},
		// `+=` with a body in it is **not** the reference's answer and is
		// not claimed to be: ksh93u+ refuses it outright, `c: invalid
		// append to associative array` at status 1, which is #2620's family
		// of rows and not modeled here. The row is a pin on the answer this
		// shell already gave before the table reading landed, so that the
		// new branch cannot quietly swallow the append too.
		{`typeset -A c=([k]=v); c+=(a=1); typeset -p c`, "typeset -C c=(a=1)\n"},
		{`typeset -A c=([k]=v); typeset -p c`, "typeset -A c=([k]=v)\n"},
		// A member of the element is enumerated under the element's name.
		{`typeset -A c=(a=1 b=2); print -r -- ${!c[0].@}`, "c[0].a c[0].b\n"},
		{`typeset -A c=(a=1 b=2); c[0].a=7; typeset -p c`, "typeset -A c=([0]=(a=7;b=2))\n"},
		{`typeset -A c=(a=1); c[z]=9; typeset -p c`, "typeset -A c=([0]=(a=1) [z]=9)\n"},
	} {
		if out, st := kshOut(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q at 0", c.src, out, st, c.want)
		}
	}
	// A bare-word literal is what the letter forbids, and it is refused where
	// it always was.
	if out, st := kshOut(t, `typeset -A c=(x y); echo tail`); st == 0 ||
		out == "" {
		t.Errorf("typeset -A c=(x y) = %q at %d, want a refusal", out, st)
	}
}

// The whole-shell listing inverts over an element. A bare compound's members
// are written inside it and nowhere else; an element's compound is already
// written whole inside the array's own row, so the *namespace* is not a row
// and its direct members are.
func TestTheWholeShellListingOverACompoundElement(t *testing.T) {
	for _, c := range []struct {
		src  string
		want []string
	}{
		{
			`a[1]=(p=1 q=2); typeset -p`,
			[]string{"typeset -a a=([1]=(p=1;q=2))", "a[1].p=1", "a[1].q=2"},
		},
		{
			// Direct members only: `a[1].q.r` is written inside `a[1].q`
			// exactly as `c.b.y` is written inside `c.b`.
			`a[1]=(p=1 q=(r=2)); typeset -p`,
			[]string{"typeset -a a=([1]=(p=1;q=(r=2)))", "a[1].p=1", "typeset -C a[1].q=(r=2)"},
		},
		{
			`typeset -A m; m[k]=(p=1); typeset -p`,
			[]string{"typeset -A m=([k]=(p=1))", "m[k].p=1"},
		},
	} {
		out, st := kshOut(t, c.src)
		if st != 0 {
			t.Errorf("%s exited %d", c.src, st)
		}
		for _, want := range c.want {
			if !containsLine(out, want) {
				t.Errorf("%s\n has no line %q\n in %q", c.src, want, out)
			}
		}
		// The namespace itself is not a row, which is the half a listing
		// that simply walked the compound marks would have got wrong.
		if containsLine(out, "typeset -C a[1]=(p=1;q=2)") {
			t.Errorf("%s writes the element's namespace as a row of its own:\n%s", c.src, out)
		}
		// And only the *direct* members are rows. `a[1].q.r` is written
		// inside `typeset -C a[1].q=(r=2)` and nowhere else, exactly as
		// `c.b.y` is written inside `c.b` — one link down the ordinary
		// rule takes over again.
		if containsLine(out, "a[1].q.r=2") {
			t.Errorf("%s writes a second-link member as a row of its own:\n%s", c.src, out)
		}
	}
	// A bare compound keeps the rule the other way round: one line, and no
	// member beside it.
	out, _ := kshOut(t, `c=(p=1 q=2); typeset -p`)
	if !containsLine(out, "typeset -C c=(p=1;q=2)") || containsLine(out, "c.p=1") {
		t.Errorf("c=(p=1 q=2); typeset -p wrote %q", out)
	}
}

// containsLine reports whether the output holds the line exactly, which a
// substring test would not: the namespace row this suite asserts is *absent*
// is a substring of the array row it asserts is present, so a Contains check
// could not tell the two apart at all.
func containsLine(out, line string) bool {
	for _, l := range splitLines(out) {
		if l == line {
			return true
		}
	}
	return false
}
