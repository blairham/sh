// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// fileTrace is the expression every case here prints: the whole array as
// `[a][b]`, with the count beside it.
//
// `${#funcfiletrace[@]}` and not `$#funcfiletrace` for the reason
// funcstack_test's `show` gives — an unsubscripted read of a dynamic array
// answers as though the name were unset (#1600), which is core rather than
// this parameter.
const fileTrace = `print -r -- "${#funcfiletrace[@]} [${(j:][:)funcfiletrace}]"`

// TestFuncfiletraceIsTheCallSiteAsAFileAndAnAbsoluteLine is the whole of what
// this parameter is for: `$functrace` writes a call made inside a function
// body as `<function>:<offset into it>`, and a handler that wants a
// *location* cannot get one back out of that. This array writes every call
// site the one way (#4470).
//
// Every row measured 2026-09-25 against zsh 5.9.2 (`/opt/homebrew/bin/zsh`,
// aarch64-apple-darwin25) under `-f`. The shell's own name stands for the top
// level of `-c` there; the harness's runner is named `zsh`, so that is what
// the rows below say — and see
// TestFuncfiletraceNamesTheShellTwoWaysAtTheTopLevelOfMinusC for the row
// where those two names come apart.
func TestFuncfiletraceIsTheCallSiteAsAFileAndAnAbsoluteLine(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, src, want string
	}{
		{
			// The issue's own snippet. zsh answers
			// `/opt/homebrew/bin/zsh:1`, which is its `$0` and line 1 of the
			// command string.
			name: "a call from the top level of -c is the shell and the line",
			src:  "f(){ " + fileTrace + " }; f",
			want: "1 [zsh:1]\n",
		},
		{
			// Two lines rather than one, so a rule that answered with a
			// constant 1 fails here.
			name: "the line is the line the call was written on",
			src:  "f(){ " + fileTrace + " }\n\n\nf",
			want: "1 [zsh:4]\n",
		},
		{
			// **The row that is not `$functrace`.** Measured, the same
			// nesting answers `[f:1][zsh:5]` there — a name and an offset of
			// 1 into `f`'s body — and `[zsh:2][zsh:5]` here, the absolute
			// line 2 of the command string. An implementation that reused
			// `$functrace` whole would write `f:1` in the first element.
			name: "a call from inside a function is the file and the absolute line",
			src:  "f(){\n  g\n}\ng(){ " + fileTrace + " }\nf",
			want: "2 [zsh:2][zsh:5]\n",
		},
		{
			// And `$functrace`'s `f:0` — a call on the definition's own line
			// — is line 1 here rather than nought, which is the same
			// discrimination read from the other end.
			name: "a call on the definition's own line is that line, not nought",
			src:  "f(){ g; }\ng(){ " + fileTrace + " }\nf",
			want: "2 [zsh:1][zsh:3]\n",
		},
		{
			name: "three deep is three entries, innermost first",
			src:  "a(){\n  b\n}\nb(){\n\n  c\n}\nc(){ " + fileTrace + " }\na",
			want: "3 [zsh:6][zsh:2][zsh:9]\n",
		},
		{
			// Not "empty because nothing is implemented" — every row above
			// proves the parameter answers. This one says the top level has
			// nothing to report, which is zsh's own answer:
			// `${#funcfiletrace}` is 0 there.
			name: "nothing called is no entries",
			src:  fileTrace,
			want: "0 []\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("funcfiletrace = %q (status %d), want %q", out, st, tc.want)
			}
		})
	}
}

// TestFuncfiletraceNamesTheFileACallWasMadeIn is the file half, and it is the
// file the call was *in* rather than the file the function came from.
//
// The two differ exactly when a function is called from somewhere other than
// where it was defined, which is every sourced library. Measured: `zsh -f -c
// '. ./lib.zsh<newline>top'` answers with the shell's own name and not
// `./lib.zsh`, so an implementation that reused the frame's own
// [interp.Frame.File] — which is where the function was defined — would name
// the library for every such call. That is what `$funcsourcetrace` is, and the
// two are measured against each other in funcsourcetrace_test.go.
func TestFuncfiletraceNamesTheFileACallWasMadeIn(t *testing.T) {
	dir := t.TempDir()
	body := "top(){ " + fileTrace + " }\nsf(){ top }\n"
	if err := os.WriteFile(filepath.Join(dir, "lib.zsh"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("a function defined in a sourced file and called outside it", func(t *testing.T) {
		out, st := runZsh(t, dir, ". ./lib.zsh\ntop")
		want := "1 [zsh:2]\n"
		if out != want || st != 0 {
			t.Errorf("funcfiletrace = %q (status %d), want %q", out, st, want)
		}
	})

	t.Run("a call made from a function that came from the file", func(t *testing.T) {
		// `sf` is defined on line 2 of lib.zsh and calls `top` on that same
		// line, so the library *is* named here — as the file the call was
		// made in. `$functrace` writes `sf:0` for the same frame, which names
		// no file at all, and that is the whole difference between the two
		// arrays in one row.
		out, st := runZsh(t, dir, ". ./lib.zsh\nsf")
		want := "2 [./lib.zsh:2][zsh:2]\n"
		if out != want || st != 0 {
			t.Errorf("funcfiletrace = %q (status %d), want %q", out, st, want)
		}
	})
}

// TestFuncfiletraceLocatesACallFromASourcedFilesTopLevel is the file case with
// a file in it, which the rows above reach only through `-c`.
//
// Measured: a file sourced at line 2 of the command string, calling `h` on its
// own line 4, answers `./src.zsh:4` for `h` and `zsh:2` for the file. And a
// `source` run from inside a function `w` answers with the file `w` was
// defined in — where `$functrace` writes `w:0` — so the rule is about where
// the call was made and not about what kind of unit was entered.
func TestFuncfiletraceLocatesACallFromASourcedFilesTopLevel(t *testing.T) {
	dir := t.TempDir()
	body := "\n\nh(){ " + fileTrace + " }\nh\n"
	if err := os.WriteFile(filepath.Join(dir, "src.zsh"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("sourced at the top level", func(t *testing.T) {
		out, st := runZsh(t, dir, "\n. ./src.zsh")
		want := "2 [./src.zsh:4][zsh:2]\n"
		if out != want || st != 0 {
			t.Errorf("funcfiletrace = %q (status %d), want %q", out, st, want)
		}
	})

	t.Run("sourced from inside a function", func(t *testing.T) {
		out, st := runZsh(t, dir, "w(){ . ./src.zsh; }\n\n\nw")
		want := "3 [./src.zsh:4][zsh:1][zsh:4]\n"
		if out != want || st != 0 {
			t.Errorf("funcfiletrace = %q (status %d), want %q", out, st, want)
		}
	})
}

// TestFuncfiletraceLocatesACallInTheScriptItself is the route the `-c` rows
// cannot reach: with a script file there is a frame at the bottom of
// [interp.Runner.CallStack], and every call is located in that script rather
// than in anything the shell calls itself.
//
// It is also the route that would catch the array growing an entry for the
// script the way bash's `FUNCNAME` grows `main`: zsh has none, so the counts
// here are the same as the `-c` ones a frame shallower.
func TestFuncfiletraceLocatesACallInTheScriptItself(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
	}{
		{
			name: "the top level of a script is no entries",
			src:  fileTrace,
			want: "0 []\n",
		},
		{
			name: "a call at the top level of a script names the script",
			src:  "g(){ " + fileTrace + " }\n\ng",
			want: "1 [/s/main.zsh:3]\n",
		},
		{
			// `$functrace` writes `f:1` for the inner frame here. This is the
			// same call site with the script named and the line absolute.
			name: "a call inside a function is the script and an absolute line",
			src:  "f(){\n  g\n}\ng(){ " + fileTrace + " }\nf",
			want: "2 [/s/main.zsh:2][/s/main.zsh:5]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := runZshScriptFile(t, tc.src, "/s/main.zsh")
			if got != tc.want {
				t.Errorf("funcfiletrace = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestFuncfiletraceIsReadonlyAndUnlisted keeps the two attributes the
// parameter carried while it was absent, because losing either is how a
// produced array stops being produced: an assignment that landed in a stored
// array would shadow the view from then on, and a listing that wrote the
// frames out would be a stack somebody could source back.
//
// Measured on zsh 5.9.2: `funcfiletrace=(a b)` is `read-only variable:
// funcfiletrace` at status 1 and `${(t)funcfiletrace}` is
// `array-readonly-hide-hideval-special`.
func TestFuncfiletraceIsReadonlyAndUnlisted(t *testing.T) {
	dir := t.TempDir()

	t.Run("an assignment is refused", func(t *testing.T) {
		out, st := runZsh(t, dir, "funcfiletrace=(a b)\nprint -r -- after")
		if st == 0 {
			t.Errorf("funcfiletrace=(a b) = %q at status 0, want a read-only refusal", out)
		}
		wantWholeLines(t, out, "zsh:1: read-only variable: funcfiletrace")
	})

	t.Run("the type word says so", func(t *testing.T) {
		out, st := runZsh(t, dir, `print -r -- "${(t)funcfiletrace}"`)
		want := "array-readonly-hide-hideval-special\n"
		if out != want || st != 0 {
			t.Errorf("${(t)funcfiletrace} = %q (status %d), want %q", out, st, want)
		}
	})
}

// TestFuncfiletraceNamesTheShellTwoWaysAtTheTopLevelOfMinusC is the row #4464
// predicted from `$functrace` and this one confirms: the two halves of "where
// am I" come apart on the `-c` route, and they come apart the same way here.
//
// Under `-c` there is no file, and zsh 5.9.2 fills the gap differently
// depending on what is being named. Measured through a symlink called
// `./myzsh`, which is what makes the rows discriminating — with the binary
// invoked by its real path several of these strings are equal and the pair
// looks like one rule:
//
//	./myzsh -f -c 'f(){ … }<newline>f'                    ./myzsh:2
//	./myzsh -f -c '<newline>. ./src.zsh'                  zsh:2
//	./myzsh -f -c 'f(){<newline>  g<newline>}<newline>g(){ … }<newline>f'
//	                                                      zsh:2 then ./myzsh:5
//
// So a **call site at the top level of `-c`** is `$0`, while the **file a
// function written there belongs to** is the shell's own fixed name — and the
// third row has both in one answer. A `$0` used throughout would write
// `./myzsh:2` for the first element of that row; the fixed name used
// throughout would write `zsh:5` for its second.
func TestFuncfiletraceNamesTheShellTwoWaysAtTheTopLevelOfMinusC(t *testing.T) {
	dir := t.TempDir()
	body := "\n\nh(){ " + fileTrace + " }\nh\n"
	if err := os.WriteFile(filepath.Join(dir, "src.zsh"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("a call made at the top level is located at $0", func(t *testing.T) {
		out, st := runZshNamed(t, dir, "./myzsh", "f(){ "+fileTrace+" }\nf")
		want := "1 [./myzsh:2]\n"
		if out != want || st != 0 {
			t.Errorf("funcfiletrace = %q (status %d), want %q", out, st, want)
		}
	})

	t.Run("a sourced file is located at the shell's own name", func(t *testing.T) {
		out, st := runZshNamed(t, dir, "./myzsh", "\n. ./src.zsh")
		want := "2 [./src.zsh:4][zsh:2]\n"
		if out != want || st != 0 {
			t.Errorf("funcfiletrace = %q (status %d), want %q", out, st, want)
		}
	})

	t.Run("both names in one answer", func(t *testing.T) {
		// The discriminating row, and the one `$functrace` cannot reach: it
		// writes `f:1` for the inner element and never asks what file a
		// function defined under `-c` belongs to.
		out, st := runZshNamed(t, dir, "./myzsh", "f(){\n  g\n}\ng(){ "+fileTrace+" }\nf")
		want := "2 [zsh:2][./myzsh:5]\n"
		if out != want || st != 0 {
			t.Errorf("funcfiletrace = %q (status %d), want %q", out, st, want)
		}
	})
}
