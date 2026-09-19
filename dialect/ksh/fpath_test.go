// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// The two marks a `-f` line puts on a function here — `-u`, which reads the
// body from `$FPATH` at the first call, and `-t`, which traces it.
//
// Measured 2026-09-15 against ksh93u+ 2012-08-01, `env -i` with a scratch
// HOME and no startup files. See dialect/ksh/fpath.go for the whole table.

// withFPath builds a directory of function files and runs src with `$FPATH`
// pointing at it.
func withFPath(t *testing.T, src string) (string, int) {
	t.Helper()
	dir := t.TempDir()
	for name, body := range map[string]string{
		// The ordinary shape: a file that defines the function it is named
		// after.
		"zz": "zz(){ echo \"zz def $1\"; }\n",
		// A file that runs commands and defines nothing.
		"bb": "echo \"bare body $1\"\n",
		// And one that defines the wrong name.
		"other": "qq(){ echo qq; }\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	out, st, err := preset.CombinedThroughTheAliases(t, dialecttest.Base{
		Dir: dir, Vars: map[string]string{"PATH": dir, "FPATH": dir},
	}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}

func TestTheUndefinedMarkReadsTheBodyFromFPath(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		{name: "marking is silent", src: `typeset -fu zz; echo "st=$?"`, want: "st=0\n"},
		{name: "and under the other word", src: `functions -u zz; echo "st=$?"`, want: "st=0\n"},
		// A name still waiting lists as a **declaration** and not a body,
		// which is this shell's own rendering and has no header and no
		// braces for the substrate's block form to go in.
		{name: "the listing is a declaration", src: `typeset -fu zz; typeset -f zz`, want: "typeset -fu zz\n"},
		{name: "and under the other word", src: `functions -u zz; functions zz`, want: "typeset -fu zz\n"},
		// A name nothing will ever find still lists, because the mark is the
		// record and the file is not read until the call.
		{name: "a name with no file lists too", src: `typeset -fu qq; typeset -f qq; echo "st=$?"`, want: "typeset -fu qq\nst=0\n"},
		// `whence` has a third answer for it, which is neither a function
		// nor nothing.
		{name: "whence says undefined", src: `typeset -fu zz; whence -v zz`, want: "zz is an undefined function\n"},
		// The call reads the file and runs what it defines.
		{name: "the call loads it", src: `typeset -fu zz; zz hello; echo "st=$?"`, want: "zz def hello\nst=0\n"},
		{name: "and the body is there afterwards", src: `typeset -fu zz; zz hi >/dev/null; typeset -f zz`, want: "zz(){ echo \"zz def $1\"; }\n"},
		{name: "and whence says so too", src: `typeset -fu zz; zz hi >/dev/null; whence -v zz`, want: "zz is a function\n"},
		// The file is read **once**: a second call runs the body it left and
		// does not source it again.
		{
			name: "a second call does not re-read", src: `typeset -fu zz; zz a; zz b`,
			want: "zz def a\nzz def b\n",
		},
		// The file is *sourced*, not read as a body — its commands run, and
		// then it has to have defined the name. This is where it parts from
		// the other shell's autoloading, which reads the file as the body.
		{
			name: "the file's own commands run", src: `typeset -fu bb; bb hello; echo "never"`,
			want: "bare body \nksh: function, built-in or type definition for bb not found in ", status: 126,
		},
		{
			name: "a file that defines the wrong name", src: `typeset -fu other; other; echo "never"`,
			want: "ksh: function, built-in or type definition for other not found in ", status: 126,
		},
		// And a name with no file at all is a different failure: 127, and
		// the script carries on.
		{
			name: "no file is 127 and not fatal", src: `typeset -fu nope; nope; echo "st=$?"`,
			want: "ksh: function: not found\nst=127\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := withFPath(t, tc.src)
			if st != tc.status {
				t.Errorf("%s = %q at %d, want %d", tc.src, out, st, tc.status)
			}
			if len(out) < len(tc.want) || out[:len(tc.want)] != tc.want {
				t.Errorf("%s = %q, want it to start %q", tc.src, out, tc.want)
			}
		})
	}
}

// `-t` traces a function, and **only** one defined with the `function` word.
//
// That is this shell's standing split between the two function forms reaching
// one more feature — `typeset` declares a local in a keyword body and assigns
// the global in a parenthesised one — and it is why the trace hook is handed
// the keyword flag rather than only the name.
func TestTheTracingMarkTracesAKeywordFunction(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		{
			name: "a keyword function traces", src: `function f { echo in; }; typeset -ft f; f; echo out`,
			want: "+ echo in\nin\nout\n",
		},
		{
			// The discriminating row: the same mark on the parenthesised
			// spelling traces nothing at all.
			name: "the parenthesised form does not", src: `f(){ echo hi; }; typeset -ft f; f; echo out`,
			want: "hi\nout\n",
		},
		{
			// The mark is one body's and does not reach what that body
			// calls: `g` is traced as a *command in f* and its own body is
			// not.
			name: "it does not reach a callee",
			src:  `function g { echo g; }; function f { g; echo f; }; typeset -ft f; f; echo out`,
			want: "+ g\ng\n+ echo f\nf\nout\n",
		},
		{name: "and it lasts past one call", src: `function f { echo hi; }; typeset -ft f; f; f`, want: "+ echo hi\nhi\n+ echo hi\nhi\n"},
		{name: "the plus form takes it off", src: `function f { echo hi; }; typeset -ft f; typeset +ft f; f; echo out`, want: "hi\nout\n"},
		{name: "and under the other word", src: `function f { echo hi; }; functions -t f; f`, want: "+ echo hi\nhi\n"},
		// The mark is not written into the listing: `typeset -f f` writes
		// the body as it always did.
		{name: "the listing is unchanged", src: `function f { echo hi; }; typeset -ft f; typeset -f f`, want: "function f { echo hi; };"},
		// The tracing mark needs a function to go on, where the autoloading
		// one makes one.
		{name: "a name that is not a function", src: `typeset -ft nosuch; echo "st=$?"`, want: "st=1\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKsh(t, dir, tc.src)
			if out != tc.want || st != tc.status {
				t.Errorf("%s = %q at %d, want %q at %d", tc.src, out, st, tc.want, tc.status)
			}
		})
	}
}

// A listing narrowed by a marking letter with no operands is the functions
// holding that mark.
//
// **The real shell writes these rows with the name missing**, which is
// recorded and deliberately not reproduced: measured, `typeset -fu zz;
// typeset -fu` is `typeset -fu ` — the letters, a space, and nothing else —
// and after the body has been loaded the same line is `(){ echo "zz def $1";
// }`, a body with no name in front of it. Two marked names give two identical
// nameless rows. A listing that loses the name it is about is a listing
// nothing can read back, and matching it would mean writing out less than
// this shell knows; the rows below write the name.
//
// It also keeps a loaded name in the `-u` set where `whence` has stopped
// calling it undefined, which is the same inconsistency from the other side.
// This engine takes the mark off at the load, so the two agree.
func TestAMarkingLetterWithNoNamesNarrowsTheListing(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the waiting names", `typeset -fu zz; typeset -fu`, "typeset -fu zz\n"},
		{"and nothing once loaded", `typeset -fu zz; zz a >/dev/null; typeset -fu; echo "st=$?"`, "st=0\n"},
		{"with none marked it is silent", `typeset -fu; echo "st=$?"`, "st=0\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := withFPath(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			"the traced names",
			`function f { echo hi; }; function g { echo bye; }; typeset -ft f; typeset -ft`,
			"function f { echo hi; };",
		},
		{
			// The control: the un-narrowed listing has both, so the row
			// above is about the mark and not about a listing that writes
			// one function.
			"against the whole table",
			`function f { echo hi; }; function g { echo bye; }; typeset -ft f; typeset -f`,
			"function f { echo hi; };function g { echo bye; };",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}
