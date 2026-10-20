// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
)

// A subscript and a length written without braces, end to end through this
// dialect. Measured 2026-09-05 on zsh 5.9.2; bash 5.3.15, bash 3.2.57 and dash
// read the same characters as a parameter followed by text, which is the
// grammar split `syntax.BareSubscript` exists for.
//
// The two shapes this once left out — a subscript on a scalar and one on the
// positional list — are answered now, in `subscripteval_test.go`. They were an
// *evaluation* gap rather than this one, present in the braced spelling too,
// and the shared span is what let one fix answer both (#854).
//
// The idiom is not an obscure one: the shell integration this machine's own
// startup files load tests `"$precmd_functions[-1]"` against a name, and
// without the form that comparison silently compared the whole array joined
// with `[-1]` stuck on the end — never equal, so the hook list was rebuilt at
// every prompt (#846).
func TestASubscriptNeedsNoBraces(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"an element", `a=(x y z); echo "[$a[1]]"`, "[x]\n"},
		{"the last element", `a=(x y z); echo "[$a[-1]]"`, "[z]\n"},
		{"below the first", `a=(x y z); echo "[$a[0]]"`, "[]\n"},
		{"past the end", `a=(x y z); echo "[$a[9]]"`, "[]\n"},
		{"a subscript that is an expression", `a=(x y z); echo "[$a[1+1]]"`, "[y]\n"},
		{"a subscript holding an expansion", `a=(x y z); i=3; echo "[$a[$i]]"`, "[z]\n"},
		{"text after the subscript", `a=(x y z); echo "[$a[1]w]"`, "[xw]\n"},
		{"one subscript and no more", `a=(x y z); echo "[$a[1][1]]"`, "[x[1]]\n"},
		{"a positional takes none", `set -- abcd; echo "[$1[2]]"`, "[abcd[2]]\n"},
		{"a comparison reads it", `a=(x y fig); [[ "$a[-1]" == fig ]] && echo last`, "last\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// `$#name` is a length here and `$#` with a letter after it everywhere else.
//
// The quiet half of the same split: both readings are a number and neither
// shell says anything, so a script testing `$#a` against zero tests the count
// in this shell and the two characters `0a` in the others.
func TestALengthNeedsNoBraces(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"an array's count", `a=(x y z); echo "[$#a]"`, "[3]\n"},
		{"an empty array", `a=(); echo "[$#a]"`, "[0]\n"},
		{"a name never set", `echo "[$#nothing]"`, "[0]\n"},
		{"a scalar's length", `s=hello; echo "[$#s]"`, "[5]\n"},
		{"an element's length", `a=(x yy zzz); echo "[$#a[3]]"`, "[3]\n"},
		{"the positional count", `set -- p q r; echo "[$#][$#@][$#*]"`, "[3][3][3]\n"},
		{"a second # is text", `set -- p q; echo "[$##]"`, "[2#]\n"},
		{"a ! after it is text", `set -- p q; echo "[$#!]"`, "[2!]\n"},
		{"inside arithmetic", `a=(x y z); echo "[$(( $#a + 1 ))]"`, "[4]\n"},
		{"as an arithmetic command", `a=(x y z); (( $#a )) && echo nonzero`, "nonzero\n"},
		{"inside a condition", `a=(x y z); [[ $#a -eq 3 ]] && echo three`, "three\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}
