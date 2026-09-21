// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"os"
	"path/filepath"
	"testing"
)

// `unset "a[@]"` empties the array, and an `@` that **arrived** through an
// expansion in the subscript does not: it is an operand the arithmetic reader
// has no value for, and the array is left standing.
//
// Measured 2026-09-21 under `env -i PATH=/usr/bin:/bin LC_ALL=C`, each case
// its own `-c`, bash 5.3.20 and 3.2.57 alike over `a=(x y)` — the two columns
// agree on every row, so this is not a version's quirk, and the two spellings
// of the arrival agree with each other, so it is the provenance of the token
// and not the substitution:
//
//	unset "a[@]"                   the array is emptied, status 0
//	w='a[$(echo @)]'; unset "$w"   `@: operand expected`, status 1
//	k=@; w='a[$k]'; unset "$w"     the same sentence
//
// The same three rows with `*` in place of `@`. An arithmetic subscript that
// will not read gives up the **script** in this shell — the `echo` written
// after the `unset` never runs in either column, which is the row the
// control one line down has and these do not — so the array standing is not
// observable from inside the run and its absence from the assertion is the
// measurement rather than an omission. See interp.Semantics.UnsetArraySpan
// for the reading itself and #4070 for the provenance.
func TestAnArrivedSpanSubscriptIsAnArithmeticOperandAndNotTheWholeArray(t *testing.T) {
	for _, sub := range []string{"@", "*"} {
		const tail = `; echo "AFTER n=${#a[@]}"`
		want := "bash: line 1: " + sub +
			": arithmetic syntax error: operand expected (error token is \"" + sub + "\")\n"
		for _, c := range []struct{ name, src string }{
			{
				"out of a substitution",
				`a=(x y); w='a[$(echo "` + sub + `")]'; unset "$w"` + tail,
			},
			{
				"out of a parameter",
				`a=(x y); k='` + sub + `'; w='a[$k]'; unset "$w"` + tail,
			},
		} {
			t.Run(sub+" "+c.name, func(t *testing.T) {
				if out, st := runBash(t, t.TempDir(), c.src); out != want || st != 1 {
					t.Errorf("%s = %q (status %d), want %q at 1", c.src, out, st, want)
				}
			})
		}
		// The control, one line away: the token the operand itself carried
		// still names every element.
		t.Run(sub+" written in the operand", func(t *testing.T) {
			src := `a=(x y); unset "a[` + sub + `]"` + tail
			if out, st := runBash(t, t.TempDir(), src); out != "AFTER n=0\n" || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", src, out, st, "AFTER n=0\n")
			}
		})
	}
}

// Nothing in a subscript is matched against the working directory on the
// operand's own account: the quoting inside a `$( … )` belongs to the
// commands in it, and taking it off here ran `echo *` where the script wrote
// `echo "*"`.
//
// Measured 2026-09-21 from a directory holding `p.sh`, `q.txt` and `r.md`,
// bash 5.3.20 and 3.2.57 alike: the quoted body is one asterisk and the
// sentence names it, where the **unquoted** body `a[$(echo *)]` is the
// listing in both columns — so the body globs, as an ordinary command does,
// and what must not happen is the subscript scan deciding for it (#4070).
func TestAnArrivedSubscriptIsNotMatchedAgainstTheDirectory(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"p.sh", "q.txt", "r.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	src := `a=(x y); w='a[$(echo "*")]'; unset "$w"; echo "AFTER n=${#a[@]}"`
	want := "bash: line 1: *: arithmetic syntax error: operand expected (error token is \"*\")\n"
	if out, st := runBash(t, dir, src); out != want || st != 1 {
		t.Errorf("%s = %q (status %d), want %q at 1", src, out, st, want)
	}
	// And the body that really was written live still globs, which is what
	// says the region is stepped over rather than switched off.
	src = `a=(x y); w='a[$(echo *)]'; unset "$w"; echo "AFTER n=${#a[@]}"`
	want = "bash: line 1: p.sh q.txt r.md: arithmetic syntax error: " +
		"invalid arithmetic operator (error token is \".sh q.txt r.md\")\n"
	if out, st := runBash(t, dir, src); out != want || st != 1 {
		t.Errorf("%s = %q (status %d), want %q at 1", src, out, st, want)
	}
}

// The quoting a substitution's body carries reaches the commands in it, so a
// key that really holds a space is found through a quoted body.
//
// Measured on bash 5.3.20 in the same sweep: with `declare -A m; m['x  y']=V`,
// `w='m[$(echo "x  y")]'; unset "$w"` empties the table. The quotes are the
// substitution's, not the subscript's, and the key is the two words with
// **two** spaces between them — which is the whole of what the assertion
// rests on, since `echo x  y` with the quotes taken off writes one.
func TestAQuotedSubstitutionBodyKeepsItsQuotingInASubscript(t *testing.T) {
	src := `declare -A m; m['x  y']=V; w='m[$(echo "x  y")]'; unset "$w"; echo "n=${#m[@]}"`
	if out, st := runBash(t, t.TempDir(), src); out != "n=0\n" || st != 0 {
		t.Errorf("%s = %q (status %d), want %q", src, out, st, "n=0\n")
	}
}
