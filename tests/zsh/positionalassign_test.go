// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// An assignment whose name is a number writes the positional parameter that
// number names, which is this shell's alone.
//
// Measured 2026-09-07 against zsh 5.9.2, and every row here is that run. The
// panel is the reason it is a grammar flag: `set -- x; 1=abc; echo $1` is
// `abc` there and `1=abc: command not found` at 127 in bash 5.3.15, that
// binary as `sh`, bash 3.2.57, dash and ksh93.
//
// In this package rather than in interp, and that is the whole point of where
// it lives: an interp test can only build a synthetic table, so it would pass
// against a preset that never turned the flag on. These rows run the real one.
//
// The rows are chosen to tell the reading apart from the ones that agree with
// it on the obvious case. A shell that stored the value under the *name* `1`
// would print `abc` for the first row and leave `$#` alone, which is why the
// count is on nearly every line; a shell that wrote the element without
// extending would answer the same everywhere the index is in range.
func TestANumberMayBeAnAssignmentsName(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The plain write, and the count that says it landed in the list.
		{`set -- x; 1=abc; print -r -- "st=$? [$1] n=$#"`, "st=0 [abc] n=1"},
		{`set -- a b c; 2=Q; print -r -- "[$*] n=$#"`, "[a Q c] n=3"},
		// With nothing set beforehand the list is created.
		{`1=abc; print -r -- "[$1] n=$#"`, "[abc] n=1"},
		// Past the end it extends, and the gap is empty parameters.
		{`set -- a b; 9=nine; print -r -- "n=$# [$9][$3]"`, "n=9 [nine][]"},
		{`set -- a b; 3=x; print -r -- "[$*] n=$#"`, "[a b x] n=3"},
		// The digits are a number, not a spelling.
		{`set -- a; 01=z; print -r -- "[$1] n=$#"`, "[z] n=1"},
		// An empty value is a value.
		{`set -- a b; 1=; print -r -- "n=$# [$1][$2]"`, "n=2 [][b]"},
		// Several in one command, applied left to right.
		{`set -- a; 1=x 2=y 3=z; print -r -- "[$*] n=$#"`, "[x y z] n=3"},
		// And beside an ordinary name, which still becomes a variable.
		{`set -- a b; 1=X foo=bar; print -r -- "[$*] foo=$foo"`, "[X b] foo=bar"},
		// `+=` joins what the parameter held, and an absent one has nothing
		// in front of the value.
		{`set -- abc; 1+=x; print -r -- "[$1]"`, "[abcx]"},
		{`1+=x; print -r -- "[$1] n=$#"`, "[x] n=1"},
		{`set -- a b; 1+=; print -r -- "[$1] n=$#"`, "[a] n=2"},
		// `0` is the shell's own name and is not one of the parameters.
		{`set -- a b; 0=abc; print -r -- "[$0] n=$#"`, "[abc] n=2"},
		{`set -- a b; 0=x; 0+=Z; print -r -- "[$0]"`, "[xZ]"},
		// The write is a write and nothing more: `shift` and `set --` move
		// over it afterwards exactly as they would over `set`'s own words.
		{`set -- a b c; 1=X; shift; print -r -- "[$*]"`, "[b c]"},
		{`set -- a b c; 1=X; set -- p q; print -r -- "[$*]"`, "[p q]"},
		// A subshell's write does not reach the shell around it.
		{`set -- a; (1=sub; print -r -- "in [$1]"); print -r -- "out [$1]"`, "in [sub]\nout [a]"},
		// A function writes its own parameters, and the caller keeps its.
		{
			`f(){ 1=inner; print -r -- "in [$1] n=$#"; }; set -- outer; f zzz; print -r -- "out [$1] n=$#"`,
			"in [inner] n=1\nout [outer] n=1",
		},
		// The value is an assignment's value: neither split nor globbed. The
		// two rows that say the difference is in the parse rather than in
		// what happens afterwards.
		{`v="a b"; set -- x; 1=$v; print -r -- "n=$# [$1]"`, "n=1 [a b]"},
		{`set -- x; 1=*; print -r -- "[$1]"`, "[*]"},
		// A command substitution on the right is still one value.
		{`set -- x; 1=$(print -r -- "p q"); print -r -- "n=$# [$1]"`, "n=1 [p q]"},
		// `set -u` has nothing to say about it: the parameter is being set.
		{`set -u; 1=abc; print -r -- "[$1] n=$#"`, "[abc] n=1"},
		// The bound on how far one assignment may extend the list, which is
		// this shell's own limit rather than the reference shell's: it grows
		// the parameters to whatever the index says, and refuses only an
		// index too wide to represent. Past the bound the assignment does
		// nothing, which is what the reference shell does for the index it
		// cannot represent. The pair is written together so the row says
		// where the edge is and not merely that there is one.
		{`1048576=x; print -r -- "n=$#"`, "n=1048576"},
		{`1048577=x; print -r -- "n=$#"`, "n=0"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s:\n  said %q\n  want %q", tc.src, got, tc.want)
		}
	}
}

// The list spelling splices: the words replace the one parameter the number
// names, so the count moves by the literal's length less one.
//
// Measured identical to `argv[N]=(…)` in the same run, which is what says the
// number is a subscript on the parameter list rather than a name of its own —
// recorded here as the reason for the shape rather than asserted, because
// `argv` as a second name for the list is not something this shell has yet.
func TestANumberedAssignmentOfAListSplices(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`set -- z y w; 1=(a b); print -r -- "[$*] n=$#"`, "[a b y w] n=4"},
		// Empty is the only spelling that shortens the list.
		{`set -- a b c; 2=(); print -r -- "[$*] n=$#"`, "[a c] n=2"},
		// Appending keeps the parameter and puts the words after *it*, which
		// is not after the last one.
		{`set -- a b; 1+=(z); print -r -- "[$*] n=$#"`, "[a z b] n=3"},
		// Past the end the padding goes in first and the words follow it.
		{`set -- a; 3=(x y); print -r -- "n=$# [$2][$3][$4]"`, "n=4 [][x][y]"},
		// The elements are words and expand as ones, which in this shell
		// means an unquoted parameter is not split: `1=($v)` places one
		// element holding `p q`, so the count goes to 2 and not to 3 even
		// though `$*` prints the same characters either way.
		{`v="p q"; set -- a b; 1=($v); print -r -- "[$*] n=$#"`, "[p q b] n=2"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s:\n  said %q\n  want %q", tc.src, got, tc.want)
		}
	}
}

// A prefix that is a number goes to *this* shell or nowhere.
//
// The three command kinds answer differently and all three were measured: a
// builtin and a function take the write and keep it, and an external command
// neither takes it nor exports it. An ordinary name's prefix does the
// opposite on the first two — it is taken back — so this is not the same rule
// wearing a different name.
func TestANumberedPrefixReachesTheShellAndNotTheChild(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The discriminating one: a builtin that does nothing to the
		// parameters, so what is left is the assignment or nothing. `shift`
		// and `set --` below both answer the same either way — the first
		// drops what was written and the second replaces it — which is why
		// this row is here and why it is first.
		{`set -- a b; 1=X true; print -r -- "[$*]"`, "[X b]"},
		{`set -- a b; 1=X shift; print -r -- "[$*]"`, "[b]"},
		{`set -- a b; 1=X set -- p q; print -r -- "[$*]"`, "[p q]"},
		// And the control: an ordinary name's prefix to the same builtin is
		// taken back, which is the rule this construct does *not* follow.
		{`set -- a b; x=1 shift; print -r -- "[$x][$*]"`, "[][b]"},
		{
			`set -- a b; f(){ print -r -- "in [$*]"; }; 1=X f y; print -r -- "out [$*]"`,
			"in [y]\nout [X b]",
		},
		{`set -- a b; 1=X /bin/echo hi; print -r -- "[$*]"`, "hi\n[a b]"},
		// And the child is shown no such name, which is the other half of
		// "nowhere" for the external case.
		{`set -- a b; 1=X /usr/bin/env | grep -c "^1=" || true`, "0"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s:\n  said %q\n  want %q", tc.src, got, tc.want)
		}
	}
}

// The two boundaries, and the ones a probe written with `local` or `typeset`
// would have mistaken for agreement.
//
// The grammar admits the digits where a *name* would stand, and every place
// that wants an *identifier* still refuses them — which this shell does with
// a complaint of its own, in the same run. And a subscript after the digits
// is not this construct at all: `1[0]=v` is a command name here too, and a
// pattern there, so the shell that calls an unmatched one an error refuses
// the line rather than running it.
func TestANumberIsAdmittedAsANameAndRefusedAsAnIdentifier(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`local 1=abc; echo after`, "local:1: not an identifier: 1"},
		{`set -- a; typeset 1=abc; echo after`, "typeset:1: not an identifier: 1"},
		{`set -- a; typeset 1=(a b); echo after`, "typeset:1: not an identifier: 1"},
		{`set -- a; export 1=x; echo after`, "export:1: not an identifier: 1"},
		{`set -- a; readonly 1; echo after`, "readonly:1: not an identifier: 1"},
		{`set -- a b c; 1[0]=v; echo after`, "no matches found: 1[0]=v"},
		{`set -- a; 1a=z; echo after`, "command not found: 1a=z"},
		// And a list where there is no list to splice into.
		{`set -- a b; 0=(x y); echo after`, "attempt to assign array value to non-array"},
	} {
		out, _ := answersRun(t, tc.src)
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s:\n  said %q\n  want it to contain %q", tc.src, out, tc.want)
		}
	}
}
