// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// **A value a typed name will not take gives up the rest of the line**, off a
// `-c` string, and the line is the one the shell is reading: an `eval` or a
// `.` in between does not stop it, and neither does it stop a bracketed
// subscript's give-up. Interp.Semantics.TypedValueFailureGivesUpTheLine
// (#6631).
//
// Measured 2026-10-15 on bash 5.3.20 and 3.2.57 from a script file, each row
// followed by `echo next $?` on the next line; the complaint is left out of
// what is compared. `d.sh` holds the row's own text and `echo in2`.
func TestATypedValueFailureGivesUpTheLine(t *testing.T) {
	for _, tc := range []struct{ src, dot, want string }{
		{"typeset -i n=5\nn=1+; echo same", "", "next 1"},
		{"typeset -i n=5\nn+=1+; echo same", "", "next 1"},
		{"typeset -i n=5\nread n m <<< \"1+ b\"; echo same", "", "next 1"},
		{"typeset -i n=5\ntypeset n=1+; echo same", "", "next 1"},
		{"typeset -i n=5\nprintf -v n %s 1+; echo same", "", "next 1"},
		{"typeset -i n=5\ngetopts x n -y 2>/dev/null; echo same", "", "next 1"},
		{"typeset -i n=5\nfor n in 1+; do echo in; done; echo same", "", "next 1"},
		{"typeset -i n=5\nf() { n=1+; echo in; }\nf; echo same", "", "next 1"},
		{"typeset -i n=5\n( n=1+; echo in ); echo same $?", "", "same 1\nnext 0"},
		{"set -o posix; typeset -i n=5\nn=1+; echo same", "", "next 1"},
		// Past borrowed text, to the line the shell itself is reading.
		{"typeset -i n\neval \"n=1+\necho in\"; echo same", "", "next 1"},
		{"typeset -i n\nf() { eval \"n=1+\"; echo f; }; f; echo same", "", "next 1"},
		{"typeset -i n\n. ./d.sh; echo same", "n=1+; echo in", "next 1"},
		{"a=(1)\neval 'echo ${a[1+]}; echo in'; echo same", "", "next 1"},
		{"a=(1)\n. ./d.sh; echo same", "echo ${a[1+]}; echo in", "next 1"},
		// The controls: these stop at the borrowed text.
		{"eval 'echo $((1/0)); echo in'; echo same", "", "same\nnext 0"},
		{"readonly r=1\neval 'r=2\necho inner'; echo same", "", "inner\nsame\nnext 0"},
	} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "d.sh"), []byte(tc.dot+"\necho in2\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		out, st, err := preset.Combined(t, dialecttest.Base{
			Name: "bash", Dir: dir, Env: []string{"PATH=/usr/bin:/bin"}, Route: interp.RouteScriptFile,
		}, tc.src+"\necho next $?\n")
		if err != nil {
			t.Fatalf("%q: %v", tc.src, err)
		}
		var kept []string
		for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
			if !strings.Contains(line, "error") && !strings.Contains(line, "division") && !strings.Contains(line, "readonly") {
				kept = append(kept, line)
			}
		}
		if got := strings.Join(kept, "\n"); got != tc.want || st != 0 {
			t.Errorf("%q:\ngot  %q at %d\nwant %q at 0", tc.src, got, st, tc.want)
		}
	}
}

// And off a `-c` string the whole string is the line: the complaint and
// nothing after it, at 1.
func TestATypedValueFailureEndsACommandString(t *testing.T) {
	for _, src := range []string{
		"typeset -i n=5\nn=1+; echo same\necho next\n",
		"typeset -i n=5\neval 'n=1+'; echo same\necho next\n",
	} {
		out, st := posixRouteStatus(t, src, interp.RouteCommandString)
		if strings.Contains(out, "same") || strings.Contains(out, "next") || st != 1 {
			t.Errorf("%q: got %q at %d, want the complaint alone at 1", src, out, st)
		}
	}
}
