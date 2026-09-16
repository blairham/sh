// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// `declare -n` — a **name reference**, which is a language feature rather
// than an option letter and is why #2553 was split out of #2412.
//
// Measured 2026-09-15 against bash 5.3.15, `env -i` with a scratch HOME and
// no startup files. Every row here was run against the real shell and this
// one side by side; interp/nameref.go holds the mechanism and
// dialect/ksh/nameref_test.go the other spelling.
func TestANameReferenceReadsAndWritesThroughTheName(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{"a read goes through", `v=1; declare -n r=v; echo "[$r]"`, "[1]\n"},
		{"a write goes through", `v=1; declare -n r=v; r=2; echo "[$v]"`, "[2]\n"},
		{"an append goes through", `v=1; declare -n r=v; r+=x; echo "[$v]"`, "[1x]\n"},
		// The target is resolved when the name is *used*, never when the
		// reference is made: the declaration here happens before `v` exists.
		{"the target may not exist yet", `declare -n r=v; v=5; echo "[$r]"`, "[5]\n"},
		{"and may never exist", `declare -n r=nope; echo "[$r]"; echo "st=$?"`, "[]\nst=0\n"},
		// Every operator reads through, which is what resolving at the store
		// rather than at the expansion buys: none of these knows about
		// references.
		{
			"every operator reads through",
			`v=abc; declare -n r=v; echo "${r:1:2} ${#r} ${r/a/Z} ${r^^} ${r:-d}"`,
			"bc 3 Zbc ABC abc\n",
		},
		{"set-ness reads through", `v=1; declare -n r=v; echo "${r+SET}"`, "SET\n"},
		{"and an unset target is unset", `declare -n r=v; echo "${r+SET}x"`, "x\n"},
		{"[[ -v ]] reads through", `v=1; declare -n r=v; [[ -v r ]] && echo yes`, "yes\n"},
		{"arithmetic reads through", `v=1; declare -n r=v; echo $(( r + 1 ))`, "2\n"},
		{"arithmetic writes through", `declare -n r=v; v=5; (( r++ )); echo "[$v]"`, "[6]\n"},
		{"read writes through", `v=1; declare -n r=v; read r <<< "zz"; echo "[$v]"`, "[zz]\n"},
		{"printf -v writes through", `v=1; declare -n r=v; printf -v r %s zz; echo "[$v]"`, "[zz]\n"},
		// Containers, in both directions and by element.
		{
			"an array is read through",
			`v=(a b c); declare -n r=v; echo "${r[1]} ${r[*]} ${#r[@]} ${!r[*]}"`,
			"b a b c 3 0 1 2\n",
		},
		{"an element is written through", `v=(a b c); declare -n r=v; r[1]=Z; echo "${v[*]}"`, "a Z c\n"},
		{"a table is read through", `declare -A m=([k]=1); declare -n r=m; echo "${r[k]}"`, "1\n"},
		// A reference to a reference resolves all the way.
		{"a chain resolves", `v=1; declare -n r=v; declare -n s=r; echo "[$s]"`, "[1]\n"},
		{"and `${!r}` is the end of it", `v=1; declare -n r=v; declare -n s=r; echo "${!s}"`, "v\n"},
		// `${!r}` is the target's *name* and not the double read the same
		// spelling means for an ordinary parameter — a double read would
		// have gone looking for a parameter called `1`.
		{"${!r} is the target name", `v=1; declare -n r=v; echo "${!r}"`, "v\n"},
		{"and the name itself is listed", `v=1; declare -n r=v; echo "${!r@}"`, "r\n"},
		// A reference may be aimed at an element.
		{"a reference to an element reads", `declare -n r="a[2]"; a=(x y z); echo "[$r]"`, "[z]\n"},
		{"and writes", `declare -n r="a[2]"; a=(x y z); r=Q; echo "${a[*]}"`, "x y Q\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runBash(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// What is **not** a read through the reference: three places where the
// reference itself is the subject.
func TestTheReferenceItselfIsTheSubject(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		// The listing writes where the reference points and never what it
		// reaches, which is why declarationOf asks the reference table
		// before any of the stores.
		{"a listing writes the reference", `v=1; declare -n r=v; declare -p r`, `declare -n r="v"` + "\n"},
		{"and the target lists as itself", `v=1; declare -n r=v; declare -p v`, `declare -- v="1"` + "\n"},
		{"a reference with no target lists bare", `declare -n r; declare -p r`, "declare -n r\n"},
		// An assignment to a reference that points nowhere **aims** it.
		{"the first value aims it", `v=1; declare -n r; r=v; echo "[$r]"`, "[1]\n"},
		{"and the listing says so", `v=1; declare -n r; r=v; declare -p r`, `declare -n r="v"` + "\n"},
		// A `for` loop re-points the reference rather than writing through
		// it, which is the same rule from the other side: the loop's
		// assignment is an assignment.
		{
			"a loop re-points it",
			`v=1; declare -n r=v; for r in x y; do :; done; declare -p r; echo "[$v]"`,
			"declare -n r=\"y\"\n[1]\n",
		},
		// `unset` takes the *target* away and `unset -n` takes the
		// reference away.
		{"unset removes the target", `v=1; declare -n r=v; unset r; echo "${v-UNSET}"`, "UNSET\n"},
		{"unset -n removes the reference", `v=1; declare -n r=v; unset -n r; echo "${v-UNSET}"`, "1\n"},
		{
			"and leaves nothing under the name",
			`v=1; declare -n r=v; unset -n r; declare -p r; echo "st=$?"`,
			"bash: line 1: declare: r: not found\nst=1\n",
		},
		// A plain declaration over a reference writes *through* it, which
		// says the letter is what makes the operand a target rather than a
		// value.
		{"a plain declaration writes through", `declare -n r=v; declare r=x; echo "$v"`, "x\n"},
		{"and re-aiming needs the letter", `v=1; declare -n r=v; declare -n r=w; w=2; echo "[$r]"`, "[2]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runBash(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// `local -n` is what the letter exists for: a function takes the name of a
// variable to fill in. The fresh binding is the whole of what `local` adds —
// the reference belongs to the call, and the caller's own name of the same
// spelling gets itself back on return.
func TestALocalNameReferenceBelongsToTheCall(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{"a function fills in its caller's variable", `f(){ local -n o=$1; o=filled; }; v=; f v; echo "[$v]"`, "[filled]\n"},
		{"declare -n does the same inside one", `f(){ declare -n o=$1; o=filled; }; v=; f v; echo "[$v]"`, "[filled]\n"},
		{"a target that does not exist is created", `f(){ local -n o=$1; o=filled; }; f nosuch; echo "[$nosuch]"`, "[filled]\n"},
		{"it reaches a caller's local", `f(){ local -n o=$1; o=Z; }; g(){ local m=q; f m; echo "[$m]"; }; g`, "[Z]\n"},
		{"and the call's own local", `f(){ local x=in; local -n o=x; o=set; echo "$x"; }; x=out; f; echo "$x"`, "set\nout\n"},
		// The reference goes away with the call, which is the row that says
		// it is an attribute of the *binding* and not of the name.
		{
			"the reference does not outlive the call",
			`f(){ local -n r=v; }; v=1; r=plain; f; declare -p r`,
			`declare -- r="plain"` + "\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runBash(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// The two refusals and the one warning, which are where this shell and ksh93
// part company — see Semantics.NamerefCycleIsRefused.
func TestWhatBashSaysAboutABadReference(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		{
			name: "a target that is not a name", src: `declare -n r=1bad; echo "st=$?"`,
			want: "bash: line 1: declare: `1bad': invalid variable name for name reference\nst=1\n",
		},
		{
			name: "a reference straight to itself", src: `declare -n r=r; echo "st=$?"`,
			want: "bash: line 1: declare: r: nameref variable self references not allowed\nst=1\n",
		},
		// A cycle through *another* reference is made here and complained
		// about at the read, which is the axis. The refusal above is the
		// core's answer and is the same in both shells.
		{
			name: "a cycle is made, not refused", src: `declare -n a=b; declare -n b=a; echo "st=$?"`,
			want: "st=0\n",
		},
		{
			name: "and the read is where it is said", src: `declare -n a=b; declare -n b=a; echo "[$a]"`,
			want: "bash: line 1: warning: a: circular name reference\n[]\n",
		},
		// The refusal is not fatal here, which is the other half of the same
		// difference: `typeset` is a special builtin in ksh93 and not in
		// this shell.
		{
			name: "a refusal does not end the script", src: `declare -n r=r; echo after`,
			want: "bash: line 1: declare: r: nameref variable self references not allowed\nafter\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runBash(t, dir, tc.src)
			if out != tc.want || st != tc.status {
				t.Errorf("%s = %q at %d, want %q at %d", tc.src, out, st, tc.want, tc.status)
			}
		})
	}
}

// A reference aimed at its **own name**, which is refused at the top level
// and taken inside a function — the two answers bash has for one spelling,
// and the shape `local -n r=r` is written for (#3048).
//
// Measured 2026-09-15 against bash 5.3.20, `env -i` with a scratch HOME and
// no startup files, every row run against the real shell and this one side by
// side. The declaration says the same sentence twice, once with the builtin's
// name in front of it and once without; each read through says it again; and
// a write says `maximum nameref depth` instead and lands on the **global**
// cell, past any local of the same name in between.
func TestAReferenceToItsOwnNameRefersOutward(t *testing.T) {
	dir := t.TempDir()
	const warn = "bash: line 1: warning: r: circular name reference\n"
	const declared = "bash: line 1: local: warning: r: circular name reference\n" + warn
	const depth = "bash: line 1: warning: r: maximum nameref depth (8) exceeded\n"
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		{
			// The whole of the issue in one line: the read reaches the
			// caller's value and the write reaches the caller's variable.
			"a read reaches out and a write lands out there",
			`r=OUTER; f(){ local -n r=r; echo "in=[$r]"; r=SET; }; f; echo "after=[$r]"`,
			declared + warn + "in=[OUTER]\n" + depth + "after=[SET]\n",
			0,
		},
		{
			// The discriminator between "the caller's cell" and "the global
			// cell", which is the reading a probe with one function cannot
			// tell apart: two callers' locals are stepped straight over.
			"it is the global cell and not the caller's",
			`r=L0; h(){ local r=L1; g; echo "h=[$r]"; }; g(){ local r=L2; f; echo "g=[$r]"; };` +
				` f(){ local -n r=r; echo "f=[$r]"; r=SET; }; h; echo "top=[$r]"`,
			declared + warn + "f=[L0]\n" + depth + "g=[L2]\nh=[L1]\ntop=[SET]\n",
			0,
		},
		{
			"an unset outer cell reads unset and is still written",
			`g(){ local r=L2; f; echo "g=[$r]"; }; f(){ local -n r=r; echo "f=[${r-UNSET}]"; r=SET; };` +
				` g; echo "top=[$r]"`,
			declared + warn + "f=[UNSET]\n" + depth + "g=[L2]\ntop=[SET]\n",
			0,
		},
		{
			// `-g` puts the reference *on* the global cell, so there is
			// nothing outside it to refer to and the loop closes: an empty
			// read, and a write that lands nowhere and is a **failed**
			// assignment — reported as the circle rather than as the depth,
			// at status 1, with the rest of the command list given up. So
			// there is no `after=` line here at all.
			"a global self reference is a closed loop",
			`r=OUTER; f(){ declare -gn r=r; echo "in=[$r]"; r=SET; }; f; echo "after=[$r]"`,
			"bash: line 1: declare: warning: r: circular name reference\n" + warn + warn +
				"in=[]\n" + warn,
			1,
		},
		{
			// The same failure from the other side. What is given up is the
			// *command list* the assignment stood in — the shape a readonly
			// reassignment already takes — and on the `-c` route these three
			// commands are one list, so nothing after the write runs at all
			// and the shell ends at 1. Written from a file, where `f` and
			// the two echoes are separate lines, bash gives up the function
			// body and prints `st=1` and `AFTER`; the file route is measured
			// in the pull request rather than here, because this harness has
			// only the one.
			"and the write that lands nowhere gives up the list",
			`f(){ declare -gn r=r; r=SET; echo NOPE; }; f; echo "st=$?"; echo AFTER`,
			"bash: line 1: declare: warning: r: circular name reference\n" + warn + warn,
			1,
		},
		{
			// A reference with nothing to point at is aimed by its first
			// assignment, and a value that is its own name aims it here —
			// the same state, reached without the `-n` operand.
			"an assignment can aim one at itself",
			`r=OUTER; f(){ local -n r; r=r; declare -p r; echo "in=[$r]"; r=SET; }; f; echo "after=[$r]"`,
			`declare -n r="r"` + "\n" + warn + "in=[OUTER]\n" + depth + "after=[SET]\n",
			0,
		},
		{
			// The top level keeps the refusal, which is the half that was
			// already right: there is no scope for the name to refer out to.
			"the top level still refuses",
			`r=OUTER; typeset -n r=r; echo "top=[$r]"; r=SET; echo "after=[$r]"`,
			"bash: line 1: typeset: r: nameref variable self references not allowed\n" +
				"top=[OUTER]\nafter=[SET]\n",
			0,
		},
		{
			// A plain `unset` says it twice and removes the binding it was
			// standing in front of rather than the cell it reads, so the
			// outer value survives. `unset -n` is the other door and is
			// silent.
			"a plain unset warns twice and leaves the outer cell",
			`r=OUTER; f(){ local -n r=r; unset r; }; f; echo "after=[${r-GONE}]"`,
			declared + warn + warn + "after=[OUTER]\n",
			0,
		},
		{
			"the declaration reports success",
			`r=OUTER; f(){ local -n r=r; echo "st=$?"; }; f`,
			declared + "st=0\n",
			0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runBash(t, dir, tc.src)
			if out != tc.want || st != tc.status {
				t.Errorf("%s = %q at %d, want %q at %d", tc.src, out, st, tc.want, tc.status)
			}
		})
	}
}

// The cell a reference lands on holds **nothing**, which is only visible once
// the reference is taken off it: `$r` goes through the reference, so a value
// left standing underneath is unreachable until `unset -n` opens the cell.
//
// Measured 2026-09-15 against bash 5.3.20, `env -i` with a scratch HOME and no
// startup files, every row run against the real shell and this one side by
// side. This shell kept the outer value in the cell instead, so `unset -n r`
// read the caller's value where bash reads an unset name (#3084).
//
// The discard is **not** a fact about scope. It happens with no function in
// sight and through `-g`, which is what says it belongs to the `n` letter
// rather than to what makes a local — the reading the first attribution of
// this bug took, and the reason the top-level and `-g` rows are here.
func TestANameReferenceEmptiesTheCellItLandsOn(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			// The report's own line. `${r-GONE}` rather than `$r` because
			// the question is whether the name is *set*, and an empty string
			// and an unset name both print nothing.
			"unset -n opens an empty cell",
			`r=OUTER; v=VEE; f(){ local -n r=v; unset -n r; echo "in=[${r-GONE}]"; }; f; echo "after=[${r-GONE}]"`,
			"in=[GONE]\nafter=[OUTER]\n",
		},
		{
			// No scope at all, so nothing was shadowed and there is nothing
			// for a scope to put back: the value is simply gone. The row
			// that decides where the rule lives.
			"the top level discards it too",
			`r=OUTER; v=VEE; declare -n r=v; unset -n r; echo "[${r-GONE}]"`,
			"[GONE]\n",
		},
		{
			// And `-g`, which is the same cell as the row above reached from
			// inside a function: no shadow is taken, and the global value
			// goes anyway — for good, since no return puts it back.
			"and a global declaration from inside a function",
			`r=OUTER; v=VEE; f(){ declare -gn r=v; unset -n r; echo "in=[${r-GONE}]"; }; f; echo "after=[${r-GONE}]"`,
			"in=[GONE]\nafter=[GONE]\n",
		},
		{
			// A reference with nothing to point at yet empties the cell just
			// the same: it is the letter and not the operand that does it.
			"a valueless declaration empties it as well",
			`r=OUTER; f(){ local -n r; unset -n r; echo "in=[${r-GONE}]"; }; f; echo "after=[${r-GONE}]"`,
			"in=[GONE]\nafter=[OUTER]\n",
		},
		{
			// The listing says the same thing from the other side — no
			// value against the name, where this shell listed one before.
			"and lists as holding nothing",
			`r=OUTER; f(){ local -n r; declare -p r; }; f`,
			"declare -n r\n",
		},
		{
			// Three deep, which is the only shape that can tell "the cell
			// this binding shadowed" from "some outer cell": with one caller
			// they are the same thing. What `unset -n` opens is *this*
			// binding's cell, so no caller's local is disturbed — `g` and
			// `h` still hold theirs and the global still holds L0.
			"and it is this binding's cell, not a caller's",
			`r=L0; v=VEE; h(){ local r=L1; g; echo "h=[$r]"; }; ` +
				`g(){ local r=L2; f; echo "g=[$r]"; }; ` +
				`f(){ local -n r=v; unset -n r; echo "f=[${r-GONE}]"; }; h; echo "top=[$r]"`,
			"f=[GONE]\ng=[L2]\nh=[L1]\ntop=[L0]\n",
		},
		{
			// What the opened cell *is*: an ordinary empty local. A write to
			// it stays in the call and does not reach the target the
			// reference used to point at, and the caller gets its own value
			// back on return.
			"the opened cell is an ordinary local",
			`r=OUTER; v=VEE; f(){ local -n r=v; unset -n r; r=NEW; echo "in=[$r] v=[$v]"; }; f; echo "after=[$r] v=[$v]"`,
			"in=[NEW] v=[VEE]\nafter=[OUTER] v=[VEE]\n",
		},
		{
			// An inherited name goes too, which a bare delete would have
			// missed: it is not in the parameter table to begin with, so the
			// environment would have gone on answering the read.
			"an inherited value goes with it",
			`export E=ENV; v=VEE; declare -n E=v; unset -n E; echo "[${E-GONE}]"`,
			"[GONE]\n",
		},
		{
			// Nothing to discard and nothing said about it, which is the
			// ordinary case the rows above are the exception to.
			"a name that held nothing is unremarkable",
			`v=VEE; declare -n r=v; unset -n r; echo "[${r-GONE}] v=[$v]"`,
			"[GONE] v=[VEE]\n",
		},
		{
			// And a **refused** declaration leaves the name exactly as it
			// found it. The discard belongs to a reference that gets made,
			// so it cannot be done before the refusals are past.
			"a refused declaration keeps the value",
			`r=OUTER; declare -n r=1bad; echo "st=$?"; unset -n r; echo "[${r-GONE}]"`,
			"bash: line 1: declare: `1bad': invalid variable name for name reference\n" +
				"st=1\n[OUTER]\n",
		},
		{
			// The self reference refused at the top level, for the same
			// reason and by the other door (#3048).
			"and so does a refused self reference",
			`r=OUTER; declare -n r=r; echo "st=$?"; unset -n r; echo "[${r-GONE}]"`,
			"bash: line 1: declare: r: nameref variable self references not allowed\n" +
				"st=1\n[OUTER]\n",
		},
		{
			// The reference is why none of this is visible in ordinary use:
			// the same name reads the target right up until `unset -n`.
			"the reference hides all of it until then",
			`r=OUTER; v=VEE; declare -n r=v; echo "[${r-GONE}]"; unset -n r; echo "[${r-GONE}]"`,
			"[VEE]\n[GONE]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runBash(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// A `-n` declaration over a name that carries an **array** is refused, and
// this column's shape of the refusal is: the array is looked at *after* the
// two refusals about the name, and the array **attribute** alone is enough —
// a bare `declare -a r` holding nothing is refused just as a filled one is.
//
// Measured 2026-09-16 against bash 5.3.20, `env -i` with a scratch HOME and
// no startup files; every row was run against the real shell and this one
// side by side and the two agreed line for line. ksh93 answers both halves
// the other way round, which is Semantics.NamerefArrayRefusal (#3103). This
// shell made the reference on every row below, so a name that still counted
// its elements read the target through `$r`.
func TestAReferenceIsRefusedOverAnArray(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		{
			// The report's own line, and it answers twice: the refusal, and
			// that the array is still standing afterwards. A fix that
			// emptied the cell on a refused declaration would have left
			// `${r-GONE}` reading GONE while `${#r[@]}` still answered 3.
			name: "an indexed array is refused and left alone",
			src:  `r=(a b c); v=VEE; declare -n r=v; echo "st=$?"; echo "after=[${r-GONE}] n=${#r[@]}"`,
			want: "bash: line 1: declare: r: reference variable cannot be an array\n" +
				"st=1\nafter=[a] n=3\n",
		},
		{
			name: "the associative kind too",
			src:  `declare -A r=([k]=KV); v=VEE; declare -n r=v; echo "st=$?"; echo "[${r[k]}]"`,
			want: "bash: line 1: declare: r: reference variable cannot be an array\n" +
				"st=1\n[KV]\n",
		},
		{
			// The **attribute** half of the axis: nothing is in `r` at all,
			// and this column refuses anyway. ksh93 takes this exact line.
			name: "a bare attribute holding nothing is enough",
			src:  `declare -a r; v=VEE; declare -n r=v; echo "st=$?"; echo "[${r-GONE}]"`,
			want: "bash: line 1: declare: r: reference variable cannot be an array\n" +
				"st=1\n[GONE]\n",
		},
		{
			name: "and the bare associative attribute as well",
			src:  `declare -A r; v=VEE; declare -n r=v; echo "st=$?"`,
			want: "bash: line 1: declare: r: reference variable cannot be an array\nst=1\n",
		},
		{
			// The valueless form, which asks no axis: both shells refuse it
			// on the attribute alone, and there is no target here for
			// another refusal to race.
			name: "a valueless declaration is refused too",
			src:  `r=(a b); declare -n r; echo "st=$?"; echo "n=${#r[@]}"`,
			want: "bash: line 1: declare: r: reference variable cannot be an array\nst=1\nn=2\n",
		},
		{
			// The **ordering** half of the axis, twice. A bad target and a
			// self reference each win a line where the name is also an
			// array; ksh93 writes the array sentence for both.
			name: "a bad target is reported ahead of the array",
			src:  `r=(a b); declare -n r=1bad; echo "st=$?"`,
			want: "bash: line 1: declare: `1bad': invalid variable name for name reference\nst=1\n",
		},
		{
			name: "and so is a self reference",
			src:  `r=(a b); declare -n r=r; echo "st=$?"`,
			want: "bash: line 1: declare: r: nameref variable self references not allowed\nst=1\n",
		},
		{
			// Inside a function the self reference is a *warning* rather
			// than a refusal (#3048), and the array refusal lands **between
			// its two halves**: the builtin's copy is written, the refusal
			// follows, and the shell's second copy never is. Measured, not
			// arranged.
			name: "the array refusal splits the self-reference warning",
			src:  `f(){ local r=(a b); local -n r=r; echo "st=$? in=[${r-GONE}]"; }; f`,
			want: "bash: line 1: local: warning: r: circular name reference\n" +
				"bash: line 1: local: r: reference variable cannot be an array\n" +
				"st=1 in=[a]\n",
		},
		{
			// Where the scope line falls. A fresh local binding holds no
			// array however loud the global is, so `local -n` over a global
			// array is taken — and `-gn`, which names the global cell
			// itself, is refused.
			name: "a fresh local binding is not the global array",
			src: `r=(a b c); v=VEE; f(){ local -n r=v; echo "in_st=$? in=[${r-GONE}]"; }; f; ` +
				`echo "after=[${r-GONE}]"`,
			want: "in_st=0 in=[VEE]\nafter=[a]\n",
		},
		{
			name: "but a global declaration reaches it",
			src:  `r=(a b c); v=VEE; f(){ declare -gn r=v; echo "st=$?"; }; f; echo "after=[${r-GONE}]"`,
			want: "bash: line 1: declare: r: reference variable cannot be an array\nst=1\nafter=[a]\n",
		},
		{
			// Three deep, which is the only shape that can tell "this
			// binding's cell" from "some outer cell": the refusal is `f`'s
			// and neither caller's local nor the global is disturbed by it.
			name: "three deep the refusal disturbs nothing outside it",
			src: `r=(G0 G1); v=VEE; h(){ local r=L1; g; echo "h=[${r-GONE}] n=${#r[@]}"; }; ` +
				`g(){ local r=(L2a L2b); f; echo "g=[${r-GONE}] n=${#r[@]}"; }; ` +
				`f(){ local r=(F0 F1); local -n r=v; echo "f st=$? [${r-GONE}] n=${#r[@]}"; }; ` +
				`h; echo "top=[${r-GONE}] n=${#r[@]}"`,
			want: "bash: line 1: local: r: reference variable cannot be an array\n" +
				"f st=1 [F0] n=2\ng=[L2a] n=2\nh=[L1] n=1\ntop=[G0] n=2\n",
		},
		{
			// The listing from the other side: the name is still an array
			// and carries no reference at all.
			name: "the refused name still lists as an array",
			src:  `r=(a b); v=VEE; declare -n r=v; declare -p r`,
			want: "bash: line 1: declare: r: reference variable cannot be an array\n" +
				`declare -a r=([0]="a" [1]="b")` + "\n",
		},
		{
			// The refusal is not fatal here, the other half of the same
			// difference: `typeset` is a special builtin in ksh93 and ends
			// the script there.
			name: "a refusal does not end the script",
			src:  `r=(a b); declare -n r=v; echo after`,
			want: "bash: line 1: declare: r: reference variable cannot be an array\nafter\n",
		},
		{
			// The *target* being an array raises no question: a reference
			// aimed at one reads and writes its elements, which is the
			// feature. Only the name being declared is asked about.
			name: "a reference aimed at an array is fine",
			src:  `v=(a b c); declare -n r=v; echo "st=$? ${r[1]} ${#r[@]}"`,
			want: "st=0 b 3\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runBash(t, dir, tc.src)
			if out != tc.want || st != tc.status {
				t.Errorf("%s = %q at %d, want %q at %d", tc.src, out, st, tc.want, tc.status)
			}
		})
	}
}

// The warning a read through a warned reference carries is **per read**, and
// an expansion is one read however many operators it carries.
//
// `${r-word}` said it twice here — once for the test that decides whether the
// word is substituted and once for the value behind it — so the function
// shape of #3048 wrote four warnings where bash writes three. The count is
// the assertion and not the text: a probe that printed them without counting
// could not tell four from three, which is the whole of this bug (#3104).
//
// Measured 2026-09-16 against bash 5.3.20, `env -i` with a scratch HOME and
// no startup files; every row was run against the real shell and this one
// side by side. ksh93 refuses this declaration outright and zsh, dash, bash
// 3.2 and BusyBox ash have no `-n` letter, so bash is the only column that
// answers.
func TestAWarnedReferenceSaysItOncePerRead(t *testing.T) {
	dir := t.TempDir()
	const warning = "warning: r: circular name reference"
	for _, tc := range []struct {
		name, read, value string
	}{
		// Three: two from the declaration, which says it with the builtin's
		// name and then again as the shell, and one from the read.
		{"a plain read", `$r`, "OUTER"},
		{"a set-ness test", `${r+S}`, "S"},
		{"a length", `${#r}`, "5"},
		// The two that said it twice. The value was right in both, so the
		// count is the only tell.
		{"a default word", `${r-GONE}`, "OUTER"},
		{"and its colon form", `${r:-D}`, "OUTER"},
		// The rest of the conditionals, which were already right and are
		// here so a fix that quietened one form cannot pass by quietening
		// all of them.
		{"an alternate word", `${r:+S}`, "S"},
		{"an assigning word", `${r=A}`, "OUTER"},
		{"and its colon form", `${r:=A}`, "OUTER"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := `r=OUTER; f(){ local -n r=r; echo "in=[` + tc.read + `]"; }; f`
			out, st := runBash(t, dir, src)
			if n := strings.Count(out, warning); n != 3 {
				t.Errorf("%s said the warning %d times, want 3\n%s", tc.read, n, out)
			}
			if want := "in=[" + tc.value + "]\n"; !strings.HasSuffix(out, want) {
				t.Errorf("%s = %q, want it to end %q", tc.read, out, want)
			}
			if st != 0 {
				t.Errorf("%s status %d, want 0", tc.read, st)
			}
		})
	}
}

// Three deep, which is the shape that discriminates: with one caller "the
// outer cell" and "the global cell" are the same thing, and the count has to
// stay at three while the value comes from past two intervening locals.
func TestTheCountHoldsThreeDeep(t *testing.T) {
	dir := t.TempDir()
	const warning = "warning: r: circular name reference"
	for _, read := range []string{`$r`, `${r-D}`, `${r:-D}`, `${r+S}`, `${#r}`} {
		src := `r=L0; h(){ local r=L1; g; }; g(){ local r=L2; f; }; ` +
			`f(){ local -n r=r; echo "in=[` + read + `]"; }; h`
		out, _ := runBash(t, dir, src)
		if n := strings.Count(out, warning); n != 3 {
			t.Errorf("%s said the warning %d times three deep, want 3\n%s", read, n, out)
		}
	}
}
