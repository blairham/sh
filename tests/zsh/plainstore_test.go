// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// TestABareAssignmentToOneOfTheShellsOwnNamesStillDoesItsWork holds the
// names a bare assignment must not store as a plain value, because the shell
// does something with what is stored there: a tie mirrors it, a letter folds
// it, a producer takes it, a freeze refuses it, an option lints it. The plain
// store in interp/plainstore.go answers every other bare assignment without
// asking, so each of these is a name it has to leave to the full road (#6423).
//
// Every row was measured on zsh 5.9.2 under `zsh -f -c`, one row per run, and
// matches what this shell wrote before the plain store existed.
func TestABareAssignmentToOneOfTheShellsOwnNamesStillDoesItsWork(t *testing.T) {
	for _, row := range []struct{ src, want string }{
		{`PATH=/a:/b; print -r -- "${path[*]}"`, "/a /b\n"},
		{`FPATH=/f1:/f2; print -r -- "${fpath[*]}"`, "/f1 /f2\n"},
		{`typeset -T FOO foo; FOO=a:b; print -r -- "${foo[*]}"`, "a b\n"},
		{`typeset -Z3 z; z=7; print -r -- "$z"`, "007\n"},
		{`typeset -L3 lft; lft=abcdef; print -r -- "[$lft]"`, "[abc]\n"},
		{`typeset -F2 fl; fl=1; print -r -- "$fl"`, "1.00\n"},
		{`typeset -i8 oct; oct=9; print -r -- "$oct"`, "8#11\n"},
		{`RANDOM=3; a=$RANDOM; RANDOM=3; b=$RANDOM; [[ $a == $b ]] && print seeded`, "seeded\n"},
		{`SECONDS=100; (( SECONDS >= 100 )) && print seconds`, "seconds\n"},
		{`LINENO=50; print -r -- "$LINENO"`, "read-only variable: LINENO\n"},
		{`IFS=:; v=a:b; print -r -- ${=v}`, "a b\n"},
		{`HISTSIZE=7; print -r -- "$HISTSIZE"`, "7\n"},
		{`OPTIND=4; print -r -- "$OPTIND"`, "4\n"},
		{`argv=x; print -r -- "$#:$1"`, "1:x\n"},
		{
			`f() { setopt localoptions warncreateglobal; newg=1; local l; l=2; }; f 2>&1; print -r -- "$newg"`,
			"f: scalar parameter newg created globally in function f\n1\n",
		},
		{
			`g() { setopt localoptions warnnestedvar; outer=2; }; h() { local outer=1; g; print -r -- $outer; }; h 2>&1`,
			"g: scalar parameter outer set in enclosing scope in function g\n2\n",
		},
		{`k() { typeset -h path; path=hid; print -r -- "$path"; }; k`, "hid\n"},
		{`psvar=one; print -r -- "${psvar[1]}"`, "one\n"},
		{`cdpath=/c; print -r -- "$CDPATH"`, "/c\n"},
		{`readonly rr=1; rr=2; print -r -- "$rr"`, "read-only variable: rr\n"},
		// A trim that publishes what it matched writes `$match` while the
		// value is expanded, and the store after it replaces that array with
		// the value. The plain store takes trims and leaves these to the
		// full road (#6353).
		{`setopt extendedglob; s=ab; match=${s#(#b)(a)}; typeset -p match`, "typeset match=b\n"},
		{`setopt extendedglob; s=ab; x=${s#(#b)(a)}; typeset -p x match`, "typeset x=b\ntypeset -a match=( a )\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), row.src)
		// The location in front of a diagnostic is the route's, not the
		// assignment's; what is compared is the sentence.
		var lines []string
		for line := range strings.SplitSeq(got, "\n") {
			if i := strings.Index(line, ": read-only variable:"); i >= 0 {
				line = line[i+2:]
			}
			lines = append(lines, line)
		}
		if got = strings.Join(lines, "\n"); got != row.want {
			t.Errorf("%s\n got %q\nwant %q", row.src, got, row.want)
		}
	}
}
