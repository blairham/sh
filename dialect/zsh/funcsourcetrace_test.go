// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// sourceTrace is the expression every case here prints: the whole array as
// `[a][b]`, with the count beside it.
//
// `${#funcsourcetrace[@]}` and not `$#funcsourcetrace` for the reason
// funcstack_test's `show` gives — an unsubscripted read of a dynamic array
// answers as though the name were unset (#1600), which is core rather than
// this parameter.
const sourceTrace = `print -r -- "${#funcsourcetrace[@]} [${(j:][:)funcsourcetrace}]"`

// TestFuncsourcetraceNamesWhereEachUnitWasDefined is the third question about
// the same frames, and the only one of the three that asks nothing about the
// caller: `$funcstack` says what the shell is *in*, `$functrace` and
// `$funcfiletrace` say where each of those was entered *from*, and this says
// where each was **written** (#4469).
//
// Every row measured 2026-09-25 against zsh 5.9.2 (`/opt/homebrew/bin/zsh`,
// aarch64-apple-darwin25) under `-f`. A function written in a `-c` string
// belongs to the shell's own fixed name, which is `zsh` and is *not* `$0` —
// see TestFuncsourcetraceNamesTheCommandStringAfterTheShellAndNotDollarZero.
func TestFuncsourcetraceNamesWhereEachUnitWasDefined(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, src, want string
	}{
		{
			// The definition is on line 1 and the call on line 1, and the
			// answer is the definition's — which is what makes this the
			// parameter it is rather than a second spelling of
			// `$funcfiletrace`.
			name: "one call is the line the function was written on",
			src:  "f(){ " + sourceTrace + " }; f",
			want: "1 [zsh:1]\n",
		},
		{
			// The call moves and the answer does not, which is the whole
			// discrimination against the other two arrays in one row:
			// `$funcfiletrace` answers `zsh:4` here.
			name: "the call site does not move it",
			src:  "f(){ " + sourceTrace + " }\n\n\nf",
			want: "1 [zsh:1]\n",
		},
		{
			name: "each frame answers for its own definition",
			src:  "f(){\n  g\n}\ng(){ " + sourceTrace + " }\nf",
			want: "2 [zsh:4][zsh:1]\n",
		},
		{
			name: "three deep is three entries, innermost first",
			src:  "a(){\n  b\n}\nb(){\n\n  c\n}\nc(){ " + sourceTrace + " }\na",
			want: "3 [zsh:8][zsh:4][zsh:1]\n",
		},
		{
			// Not "empty because nothing is implemented" — every row above
			// proves the parameter answers. This one says the top level has
			// nothing to report, which is zsh's own answer:
			// `${#funcsourcetrace}` is 0 there.
			name: "nothing called is no entries",
			src:  sourceTrace,
			want: "0 []\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("funcsourcetrace = %q (status %d), want %q", out, st, tc.want)
			}
		})
	}
}

// TestFuncsourcetraceCountsFromTheDeclarationWhicheverSpellingItIs holds the
// two ways a function can be written to the same line, and holds that line
// against a body that is nowhere near it.
//
// Measured on zsh 5.9.2 over a script file: `f()` on line 2 with its `{` on
// line 3 answers `:2`, `function k {` on line 6 answers `:6`, and a body
// opening with blank lines does not move either. So the line is where the
// *declaration* starts, which is the same place `$LINENO` counts a function's
// offsets from here — one fact rather than two.
func TestFuncsourcetraceCountsFromTheDeclarationWhicheverSpellingItIs(t *testing.T) {
	t.Run("both spellings", func(t *testing.T) {
		src := "g(){ " + sourceTrace + " }\nf()\n{\n  g\n}\nfunction k {\n  g\n}\nf\nk"
		got := runZshScriptFile(t, src, "/s/main.zsh")
		want := "2 [/s/main.zsh:1][/s/main.zsh:2]\n2 [/s/main.zsh:1][/s/main.zsh:6]\n"
		if got != want {
			t.Errorf("funcsourcetrace = %q, want %q", got, want)
		}
	})

	t.Run("a body of blank lines does not move it", func(t *testing.T) {
		src := "\n\n\n\ng(){\n\n  " + sourceTrace + "\n}\ng"
		got := runZshScriptFile(t, src, "/s/main.zsh")
		want := "1 [/s/main.zsh:5]\n"
		if got != want {
			t.Errorf("funcsourcetrace = %q, want %q", got, want)
		}
	})

	t.Run("a definition nested inside another function", func(t *testing.T) {
		src := "outer(){\n  inner(){ " + sourceTrace + " }\n  inner\n}\nouter"
		got := runZshScriptFile(t, src, "/s/main.zsh")
		want := "2 [/s/main.zsh:2][/s/main.zsh:1]\n"
		if got != want {
			t.Errorf("funcsourcetrace = %q, want %q", got, want)
		}
	})
}

// TestFuncsourcetraceNamesTheFileAFunctionCameFrom is the `${BASH_SOURCE[0]}`
// idiom this parameter exists for — "find the file I was loaded from" — and
// it is the one array of the three that reads the frame's **own** file.
//
// The three come apart on exactly this: a function defined in a sourced
// library and called from outside it names the library here, and names where
// the *call* was in the other two. Measured both ways on zsh 5.9.2. tig's
// shipped zsh completion has no other way to reach its own directory, which is
// why an absent parameter stopped it at the moment somebody pressed Tab.
func TestFuncsourcetraceNamesTheFileAFunctionCameFrom(t *testing.T) {
	dir := t.TempDir()
	body := "top(){ " + sourceTrace + " }\nsf(){ top }\n"
	if err := os.WriteFile(filepath.Join(dir, "lib.zsh"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("a function defined in a sourced file and called outside it", func(t *testing.T) {
		// `$functrace` and `$funcfiletrace` both answer `zsh:2` for this
		// frame — where the call was. The library is named only here.
		out, st := runZsh(t, dir, ". ./lib.zsh\ntop")
		want := "1 [./lib.zsh:1]\n"
		if out != want || st != 0 {
			t.Errorf("funcsourcetrace = %q (status %d), want %q", out, st, want)
		}
	})

	t.Run("two functions from the file, each with its own line", func(t *testing.T) {
		out, st := runZsh(t, dir, ". ./lib.zsh\nsf")
		want := "2 [./lib.zsh:1][./lib.zsh:2]\n"
		if out != want || st != 0 {
			t.Errorf("funcsourcetrace = %q (status %d), want %q", out, st, want)
		}
	})
}

// TestFuncsourcetraceWritesNoughtForASourcedFile is the detail that is only
// visible if you go looking, and it is measured rather than reasoned: a file
// has no definition line and zsh writes a literal **nought** there — not 1,
// and not the line it was sourced at.
//
// Nothing in this shell writes that nought. [interp.Frame.FuncLine] is zero on
// a frame that is not a function, so the answer is arrived at rather than
// cased — which is why the nested row below costs nothing to hold: a file
// sourced from inside another sourced file is `:0` at both depths, and the
// line each was sourced at is elsewhere in `$funcfiletrace`.
func TestFuncsourcetraceWritesNoughtForASourcedFile(t *testing.T) {
	dir := t.TempDir()
	body := "\n\nh(){ " + sourceTrace + " }\nh\n"
	if err := os.WriteFile(filepath.Join(dir, "src.zsh"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	outer := "\n\n. ./src.zsh\n"
	if err := os.WriteFile(filepath.Join(dir, "outer.zsh"), []byte(outer), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("sourced at the top level", func(t *testing.T) {
		out, st := runZsh(t, dir, "\n. ./src.zsh")
		want := "2 [./src.zsh:3][./src.zsh:0]\n"
		if out != want || st != 0 {
			t.Errorf("funcsourcetrace = %q (status %d), want %q", out, st, want)
		}
	})

	t.Run("sourced from inside a function", func(t *testing.T) {
		// The function `w` is written at line 1 of the command string and the
		// file it sourced is still `:0`, so the nought belongs to the kind of
		// unit and not to the depth.
		out, st := runZsh(t, dir, "w(){ . ./src.zsh; }\n\n\nw")
		want := "3 [./src.zsh:3][./src.zsh:0][zsh:1]\n"
		if out != want || st != 0 {
			t.Errorf("funcsourcetrace = %q (status %d), want %q", out, st, want)
		}
	})

	t.Run("a file sourced from inside another sourced file", func(t *testing.T) {
		out, st := runZsh(t, dir, "\n. ./outer.zsh")
		want := "3 [./src.zsh:3][./src.zsh:0][./outer.zsh:0]\n"
		if out != want || st != 0 {
			t.Errorf("funcsourcetrace = %q (status %d), want %q", out, st, want)
		}
	})
}

// TestFuncsourcetraceNamesTheScriptWhenThereIsOne is the route the `-c` rows
// cannot reach, and the one that would catch the array growing an entry for
// the script the way bash's `FUNCNAME` grows `main`: zsh has none.
func TestFuncsourcetraceNamesTheScriptWhenThereIsOne(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
	}{
		{
			name: "the top level of a script is no entries",
			src:  sourceTrace,
			want: "0 []\n",
		},
		{
			name: "a function the script defined names the script",
			src:  "g(){ " + sourceTrace + " }\n\ng",
			want: "1 [/s/main.zsh:1]\n",
		},
		{
			name: "two frames, each at its own definition",
			src:  "f(){\n  g\n}\ng(){ " + sourceTrace + " }\nf",
			want: "2 [/s/main.zsh:4][/s/main.zsh:1]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := runZshScriptFile(t, tc.src, "/s/main.zsh")
			if got != tc.want {
				t.Errorf("funcsourcetrace = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestFuncsourcetraceNamesTheCommandStringAfterTheShellAndNotDollarZero is the
// discriminating row for this array's half of the `-c` split, and it needs the
// symlink for the same reason `$functrace`'s did: with the binary invoked by
// its own path the two strings are equal and a wrong rule passes.
//
// Measured 2026-09-25 on zsh 5.9.2 through a symlink called `./myzsh`:
//
//	./myzsh -f -c 'f(){ … }<newline>f'    funcsourcetrace  zsh:1
//	                                      funcfiletrace    ./myzsh:2
//
// One invocation, one frame, and the two arrays name the shell differently —
// the *defining file* of a function written in a `-c` string is the fixed
// name, and the *call site* at that top level is `$0`. An implementation that
// read [interp.Frame.File] without [interp.Frame.NoFile] would write `./myzsh`
// here, because the core parks `$0` in that field for a unit that came from no
// file and that is the measured answer for `${BASH_SOURCE[@]}` in the dialect
// next door.
func TestFuncsourcetraceNamesTheCommandStringAfterTheShellAndNotDollarZero(t *testing.T) {
	dir := t.TempDir()
	out, st := runZshNamed(t, dir, "./myzsh", "f(){ "+sourceTrace+" }\nf")
	want := "1 [zsh:1]\n"
	if out != want || st != 0 {
		t.Errorf("funcsourcetrace = %q (status %d), want %q", out, st, want)
	}
}

// TestFuncsourcetraceIsReadonlyAndUnlisted keeps the two attributes the
// parameter carried while it was absent, for the reason
// TestFuncfiletraceIsReadonlyAndUnlisted gives.
//
// Measured on zsh 5.9.2: `funcsourcetrace=(a b)` is `read-only variable:
// funcsourcetrace` at status 1 and `${(t)funcsourcetrace}` is
// `array-readonly-hide-hideval-special`.
func TestFuncsourcetraceIsReadonlyAndUnlisted(t *testing.T) {
	dir := t.TempDir()

	t.Run("an assignment is refused", func(t *testing.T) {
		out, st := runZsh(t, dir, "funcsourcetrace=(a b)\nprint -r -- after")
		if st == 0 {
			t.Errorf("funcsourcetrace=(a b) = %q at status 0, want a read-only refusal", out)
		}
		wantWholeLines(t, out, "zsh:1: read-only variable: funcsourcetrace")
	})

	t.Run("the type word says so", func(t *testing.T) {
		out, st := runZsh(t, dir, `print -r -- "${(t)funcsourcetrace}"`)
		want := "array-readonly-hide-hideval-special\n"
		if out != want || st != 0 {
			t.Errorf("${(t)funcsourcetrace} = %q (status %d), want %q", out, st, want)
		}
	})
}
