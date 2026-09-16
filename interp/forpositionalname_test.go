// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A loop variable that is a positional parameter's number writes the
// parameter (#3040).
//
// The grammar half is [syntax.Dialect.ForNameMayBeAPositionalParameter]; this
// is what the loop then *does*, which is a separate answer and was wrong in
// three ways while the header parsed. Measured 2026-09-15 on zsh 5.9.2, each
// probe in a script file of its own:
//
//	set -- p q; for 1 in a b; do :; done; echo "[$1][$2]"   →  [b][q]
//	for 0 in a; do echo "[$0]"; done                        →  [a]
//	for 01 in a; do echo "[$1]"; done                       →  [a]
//	set -- a; for 9 in z; do :; done; echo $#               →  9
//	set -- p q r; for 1; do echo "[$1]"; done; echo "<$@>"  →  [p][q][r]<r q r>
//
// The last row is the one a straightforward reading gets wrong: the loop
// walks the parameters *and* writes one of them, so the list has to be taken
// once at the top. Reading it live made every pass print the first parameter
// over again.
//
// Every row asserts what is left behind as well as what the body printed. A
// loop that wrote an ordinary variable named `1` would print the same words
// on every pass here and leave the parameters untouched, which is exactly the
// reading being ruled out.

func forPositionalNameGrammar(d *syntax.Dialect) {
	d.ForNameMayBeAPositionalParameter = true
	// One row names two variables at once, which is a flag of its own.
	d.ForMultipleNames = true
}

func runForPositionalName(t *testing.T, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, forPositionalNameGrammar, func(*Runner) {})
}

func TestALoopVariableThatIsANumberWritesThePositionalParameter(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
	}{
		{
			// The parameter itself is written, and only that one.
			"the loop leaves its last value in the parameter",
			`set -- p q; for 1 in a b; do :; done; printf '[%s][%s]\n' "$1" "$2"`,
			"[b][q]\n",
		},
		{
			// And the body reads it back as the parameter, pass by pass.
			"the body reads the parameter it set",
			`for 1 in a b; do printf '[%s]' "$1"; done; printf '\n'`,
			"[a][b]\n",
		},
		{
			// Nought is the shell's own name rather than one of the list.
			"nought is the shell's name",
			`set -- p; for 0 in a; do printf '[%s]' "$0"; done; printf '%s\n' "$#"`,
			"[a]1\n",
		},
		{
			// The digits are read as a number, so a leading zero is the
			// first parameter and not a tenth one.
			"a leading zero is the same parameter",
			`for 01 in a; do :; done; printf '[%s]\n' "$1"`,
			"[a]\n",
		},
		{
			// A number past the end extends the list, exactly as the
			// assignment spelling does.
			"a number past the end extends the list",
			`set -- a; for 9 in z; do :; done; printf '%s\n' "$#"`,
			"9\n",
		},
		{
			// The row a live read gets wrong: the loop walks the parameters
			// and writes one of them as it goes.
			"a loop over the parameters that writes one of them",
			`set -- p q r; for 1; do printf '[%s]' "$1"; done; printf '<%s>\n' "$*"`,
			"[p][q][r]<r q r>\n",
		},
		{
			// Two names at once, one a number and one not, so the stride and
			// the store are asked together.
			"a number beside an ordinary name",
			`for 1 v in a b c d; do printf '[%s:%s]' "$1" "$v"; done; printf '\n'`,
			"[a:b][c:d]\n",
		},
		{
			// The control: an ordinary loop variable is still an ordinary
			// variable and leaves the parameters alone.
			"an ordinary name touches no parameter",
			`set -- p q; for i in a b; do :; done; printf '[%s][%s][%s]\n' "$i" "$1" "$2"`,
			"[b][p][q]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := runForPositionalName(t, tc.src)
			if status != 0 {
				t.Fatalf("%s: status %d, want 0 (output %q)", tc.src, status, out)
			}
			if out != tc.want {
				t.Errorf("%s: %q, want %q", tc.src, out, tc.want)
			}
		})
	}
}
