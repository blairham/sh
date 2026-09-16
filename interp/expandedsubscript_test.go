// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A subscript is a *word*, and the caller expands it before anything
// arithmetic happens. So the text that reaches the expression reader has had
// its substitutions performed once already, and performing them again runs
// what the first round only produced.
//
// This is the half that matters, because it is the half that executes
// something the script never asked to run. Measured 2026-09-15 from a script
// file: `k='$(cmd)'; a[$k]=V` is an arithmetic failure in bash 5.3.20, bash
// 3.2.57, zsh 5.9.2 and ksh93u+ 2012-08-01, and none of the four runs cmd.
// Every dialect here ran it and assigned the element its output named
// (#3047).
func TestAnExpandedSubscriptRunsNothing(t *testing.T) {
	dir := t.TempDir()
	ran := filepath.Join(dir, "ran")
	src := `k='$(echo x > ran; echo 1)'; a=(9 8 7); a[$k]=V; printf "%s" "${a[*]}"`
	out := runIn(t, dir, src)
	if _, err := os.Stat(ran); err == nil {
		t.Errorf("the command substitution in the subscript ran; %q", out)
	}
	if strings.Contains(out, "V") {
		t.Errorf("got %q, want the element left alone", out)
	}
}

// The same for a substring's range, which is the other text a caller hands
// over already expanded. `x=abcdef; w='$(echo 1)'; ${x:$w:2}` is the same
// arithmetic failure in every shell measured.
func TestAnExpandedRangeRunsNothing(t *testing.T) {
	dir := t.TempDir()
	ran := filepath.Join(dir, "ran")
	out := runIn(t, dir, `x=abcdef; w='$(echo x > ran; echo 1)'; printf "[%s]" "${x:$w:2}"`)
	if _, err := os.Stat(ran); err == nil {
		t.Errorf("the command substitution in the range ran; %q", out)
	}
}

// Nothing with a `$` or a backquote still in it begins an operand there. The
// four are one rule and not four: the text is a *result*, so the characters
// in it are characters. Measured 2026-09-15 and unanimous in bash 5.3.20,
// bash 3.2.57, zsh 5.9.2 and ksh93u+ — each is refused for wanting an
// operand, and the element is left as it was.
func TestAnExpandedSubscriptIsNotExpandedASecondTime(t *testing.T) {
	for _, tc := range []struct{ name, k string }{
		{"a command substitution", `'$(echo 1)'`},
		{"a backquoted run", "'`echo 1`'"},
		{"a nested arithmetic expansion", `'$((1))'`},
		// The mildest of them, and the one that says this is about the `$`
		// rather than about running something: `i` is set and still is not
		// read, because `$i` was already expanded once and came out as these
		// two characters.
		{"a bare parameter", `'$i'`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := `i=2; k=` + tc.k + `; a=(9 8 7 6); a[$k]=V; printf "%s" "${a[*]}"`
			out, _ := run(t, src, nil)
			if strings.Contains(out, "V") {
				t.Errorf("got %q, want the subscript refused and the element left alone", out)
			}
		})
	}
}

// And what must keep working, which is the whole reason this is a *reading*
// rather than a refusal: the text is still an expression, and a name in it is
// still resolved. Resolving a name is the evaluator's job — it is not a second
// round of expansion, and that is the distinction the fix draws. `k='i'` and
// `k='1+1'` each name an element in all four shells measured.
func TestAnExpandedSubscriptIsStillAnExpression(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"arithmetic in the expanded text", `k='1+1'; a=(9 8 7 6); a[$k]=V; printf "%s" "${a[*]}"`, "9 8 V 6"},
		{"a bare name", `i=2; k='i'; a=(9 8 7 6); a[$k]=V; printf "%s" "${a[*]}"`, "9 8 V 6"},
		{"a name in an expression", `i=1; k='i+1'; a=(9 8 7 6); a[$k]=V; printf "%s" "${a[*]}"`, "9 8 V 6"},
		{"a plain numeral", `k='3'; a=(9 8 7 6); a[$k]=V; printf "%s" "${a[*]}"`, "9 8 7 V"},
		// The source's own command substitution is untouched: it is a word
		// being expanded for the first time, which is what every shell does.
		{"a substitution the source wrote", `a=(9 8 7 6); a[$(echo 2)]=V; printf "%s" "${a[*]}"`, "9 8 V 6"},
		// The range reads the same way.
		{"an expanded range", `x=abcdef; w='1+1'; printf "[%s]" "${x:$w:2}"`, "[cd]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, _ := run(t, tc.src, nil); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// The boundary this draws, and the half that is *not* "a subscript is never
// expanded": a subscript that arrived as text rather than as a word — a
// builtin's operand, a reference resolved at run time — still has its
// expansions performed, because they have not been performed yet. The quotes
// are what kept the `$` from the word expansion that would otherwise have
// reached it.
//
// Measured on zsh 5.9.2 and recorded at #1852: `i=2; v='x[$i]'` reads the
// second element. Reading these as results instead would have been the same
// fault pointing the other way.
func TestAReferenceSubscriptIsStillExpandedOnce(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"unset through a quoted operand",
			`b=(x y z); j=1; unset 'b[$j]'; printf "%s" "${b[*]}"`, "x z",
		},
		{
			"an expression in a quoted operand",
			`b=(x y z w); j=1; unset 'b[$j+1]'; printf "%s" "${b[*]}"`, "x y w",
		},
		{
			"read through a subscripted operand",
			"a=(9 8 7 6); read 'a[1+1]' <<IN\nQ\nIN\nprintf \"%s\" \"${a[*]}\"", "9 8 Q 6",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, _ := run(t, tc.src, nil); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
