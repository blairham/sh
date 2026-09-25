// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// trace is the expression every case here prints: the whole array as `[a][b]`,
// with the count beside it.
//
// `${#functrace[@]}` and not `$#functrace` for the reason funcstack_test's
// `show` gives — an unsubscripted read of a dynamic array answers as though
// the name were unset (#1600), which is core rather than this parameter.
const trace = `print -r -- "${#functrace[@]} [${(j:][:)functrace}]"`

// TestFunctraceNamesTheCallerAndNotTheCallee is the whole of what a tracing
// handler reads this for: `$funcstack[1]` is where it *is* and
// `$functrace[1]` is where it was entered *from*.
//
// Every row measured 2026-09-25 against zsh 5.9.2 (`/opt/homebrew/bin/zsh`,
// aarch64-apple-darwin25) under `-f`. The shell's own name stands for the
// top level of `-c` there, which is `$0` — the harness's runner is named
// `zsh`, so that is what the rows below say.
func TestFunctraceNamesTheCallerAndNotTheCallee(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, src, want string
	}{
		{
			// The issue's own snippet. zsh answers
			// `/opt/homebrew/bin/zsh:1`, which is its `$0` and line 1 of the
			// command string.
			name: "a call from the top level of -c is the shell and the line",
			src:  "f(){ " + trace + " }; f",
			want: "1 [zsh:1]\n",
		},
		{
			// Two lines rather than one, so a rule that answered with a
			// constant 1 fails here.
			name: "the line is the line the call was written on",
			src:  "f(){ " + trace + " }\n\n\nf",
			want: "1 [zsh:4]\n",
		},
		{
			// The caller is a *function*, so it is named rather than located
			// in a file — and the number is the offset into its body, not
			// the line of the script.
			name: "a call from inside a function is the function and the offset",
			src:  "f(){\n  g\n}\ng(){ " + trace + " }\nf",
			want: "2 [f:1][zsh:5]\n",
		},
		{
			// The offset counts from the line the *definition* was written
			// on, so a call on that same line is `:0` — which is also the
			// answer the issue's nesting produces and the one a naive
			// "line within the body" rule gets wrong.
			name: "a call on the definition's own line is offset nought",
			src:  "f(){ g; }\ng(){ " + trace + " }\nf",
			want: "2 [f:0][zsh:3]\n",
		},
		{
			name: "three deep is three entries, innermost first",
			src:  "a(){\n  b\n}\nb(){\n\n  c\n}\nc(){ " + trace + " }\na",
			want: "3 [b:2][a:1][zsh:9]\n",
		},
		{
			// Not "empty because nothing is implemented" — every row above
			// proves the parameter answers. This one says the top level has
			// nothing to report, which is zsh's own answer: `${#functrace}`
			// is 0 there.
			name: "nothing called is no entries",
			src:  trace,
			want: "0 []\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("functrace = %q (status %d), want %q", out, st, tc.want)
			}
		})
	}
}

// TestFunctraceCountsFromTheDefinitionWhicheverSpellingItIs holds the two
// ways a function can be written to the same offset.
//
// Measured on zsh 5.9.2: `f()` on line 2 with its `{` on line 3 and the call
// on line 4 is `f:2`, and `function k {` on line 6 with the call on line 7 is
// `k:1`. So the line counted from is the line the *declaration* starts on and
// not the brace's, which is also where `$LINENO` counts from here.
func TestFunctraceCountsFromTheDefinitionWhicheverSpellingItIs(t *testing.T) {
	dir := t.TempDir()
	src := "g(){ " + trace + " }\nf()\n{\n  g\n}\nfunction k {\n  g\n}\nf\nk"
	out, st := runZsh(t, dir, src)
	want := "2 [f:2][zsh:9]\n2 [k:1][zsh:10]\n"
	if out != want || st != 0 {
		t.Errorf("functrace = %q (status %d), want %q", out, st, want)
	}
}

// TestFunctraceNamesTheFileACallWasMadeIn is the second field, and it is the
// file the call was *in* rather than the file the function came from.
//
// The two differ exactly when a function is called from somewhere other than
// where it was defined, which is every sourced library. Measured: `zsh -f -c
// '. ./lib.zsh<newline>f'` answers with the shell's own name and not
// `./lib.zsh`, so an implementation that reused the frame's own
// [interp.Frame.File] — which is where the function was defined — would name
// the library for every such call.
func TestFunctraceNamesTheFileACallWasMadeIn(t *testing.T) {
	dir := t.TempDir()
	body := "top(){ " + trace + " }\nsf(){ top }\n"
	if err := os.WriteFile(filepath.Join(dir, "lib.zsh"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("a function defined in a sourced file and called outside it", func(t *testing.T) {
		out, st := runZsh(t, dir, ". ./lib.zsh\ntop")
		want := "1 [zsh:2]\n"
		if out != want || st != 0 {
			t.Errorf("functrace = %q (status %d), want %q", out, st, want)
		}
	})

	t.Run("a call made from a function that came from the file", func(t *testing.T) {
		// `sf` is defined on line 2 of lib.zsh and calls `top` on that same
		// line, so the offset is nought — and the file it came from is not
		// named anywhere, because neither frame was entered from it.
		out, st := runZsh(t, dir, ". ./lib.zsh\nsf")
		want := "2 [sf:0][zsh:2]\n"
		if out != want || st != 0 {
			t.Errorf("functrace = %q (status %d), want %q", out, st, want)
		}
	})
}

// TestFunctraceLocatesACallFromASourcedFilesTopLevel is the file case with a
// file in it, which the rows above reach only through `-c`.
//
// Measured: a file sourced at line 2 of the script, calling `h` on its own
// line 3, answers `./src.zsh:3` for `h` and `main.zsh:2` for the file — the
// caller's file and an *absolute* line in it, where a function caller would
// have been an offset. And a `source` run from inside a function `w` reports
// `w:0`, so the rule is about what made the call and not about what kind of
// unit was entered.
func TestFunctraceLocatesACallFromASourcedFilesTopLevel(t *testing.T) {
	dir := t.TempDir()
	body := "\n\nh(){ " + trace + " }\nh\n"
	if err := os.WriteFile(filepath.Join(dir, "src.zsh"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("sourced at the top level", func(t *testing.T) {
		out, st := runZsh(t, dir, "\n. ./src.zsh")
		want := "2 [./src.zsh:4][zsh:2]\n"
		if out != want || st != 0 {
			t.Errorf("functrace = %q (status %d), want %q", out, st, want)
		}
	})

	t.Run("sourced from inside a function", func(t *testing.T) {
		out, st := runZsh(t, dir, "w(){ . ./src.zsh; }\n\n\nw")
		want := "3 [./src.zsh:4][w:0][zsh:4]\n"
		if out != want || st != 0 {
			t.Errorf("functrace = %q (status %d), want %q", out, st, want)
		}
	})
}

// TestFunctraceLocatesACallInTheScriptItself is the route the `-c` rows
// cannot reach: with a script file there is a frame at the bottom of
// [interp.Runner.CallStack], and the file a top-level call is located in is
// that script rather than the shell's name.
//
// It is also the route that would catch the array growing an entry for the
// script the way bash's `FUNCNAME` grows `main`: zsh has none, so the counts
// here are the same as the `-c` ones a frame shallower.
func TestFunctraceLocatesACallInTheScriptItself(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
	}{
		{
			name: "the top level of a script is no entries",
			src:  trace,
			want: "0 []\n",
		},
		{
			name: "a call at the top level of a script names the script",
			src:  "g(){ " + trace + " }\n\ng",
			want: "1 [/s/main.zsh:3]\n",
		},
		{
			name: "a call inside a function is still the function and an offset",
			src:  "f(){\n  g\n}\ng(){ " + trace + " }\nf",
			want: "2 [f:1][/s/main.zsh:5]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := runZshScriptFile(t, tc.src, "/s/main.zsh")
			if got != tc.want {
				t.Errorf("functrace = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestTheFourStackArraysAreTheSameLength is the invariant that makes the set
// usable at all: a handler reads `$funcstack[1]` beside `$functrace[1]`,
// `$funcfiletrace[1]` and `$funcsourcetrace[1]`, so an array that selected a
// different set of frames would line one up against the wrong other.
//
// All four rather than the first two (#4470, #4469), because that is what the
// shared walk is for — traceEntries picks the frames once and the three
// callers say only how to *write* an element, so this cannot fail without the
// selection itself being wrong. Measured equal on zsh 5.9.2 in every shape
// here.
//
// Held as an equality across shapes rather than as a fixed number, so it
// still means something when the stack gains a kind of frame. The startup
// row is the one that would have broken it — a frame the *shell* entered is
// in neither, and a walk that reported a caller for every frame would have
// made these arrays one longer than `$funcstack` at the top level of an rc
// file.
func TestTheFourStackArraysAreTheSameLength(t *testing.T) {
	dir := t.TempDir()
	both := `print -r -- "${#funcstack[@]} ${#functrace[@]} ` +
		`${#funcfiletrace[@]} ${#funcsourcetrace[@]}"`
	file := "h(){ " + both + " }\nh\n"
	if err := os.WriteFile(filepath.Join(dir, "both.zsh"), []byte(file), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, src, want string
	}{
		{name: "top level", src: both, want: "0 0 0 0\n"},
		{name: "one call", src: "f(){ " + both + " }; f", want: "1 1 1 1\n"},
		{name: "two calls", src: "f(){ g; }; g(){ " + both + " }; f", want: "2 2 2 2\n"},
		{
			name: "a subshell keeps the frames it is under",
			src:  "f(){ ( g ); }; g(){ " + both + " }; f",
			want: "2 2 2 2\n",
		},
		{
			// A sourced file is a unit in all four, and it is the shape
			// where the walk has two kinds of frame to keep in step.
			name: "a sourced file counts in all four",
			src:  "w(){ . ./both.zsh; }\nw",
			want: "3 3 3 3\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("lengths = %q (status %d), want %q", out, st, tc.want)
			}
		})
	}
}

// TestFunctraceIsReadonlyAndUnlisted keeps the two attributes the parameter
// carried while it was absent, because losing either is how a produced array
// stops being produced: an assignment that landed in a stored array would
// shadow the view from then on, and a listing that wrote the frames out would
// be a stack somebody could source back.
//
// Measured on zsh 5.9.2: `functrace=(a b)` is `read-only variable: functrace`
// at status 1, `${(t)functrace}` is `array-readonly-hide-hideval-special`,
// and `typeset -p functrace` writes nothing at all.
func TestFunctraceIsReadonlyAndUnlisted(t *testing.T) {
	dir := t.TempDir()

	t.Run("an assignment is refused", func(t *testing.T) {
		out, st := runZsh(t, dir, "functrace=(a b)\nprint -r -- after")
		if st == 0 {
			t.Errorf("functrace=(a b) = %q at status 0, want a read-only refusal", out)
		}
		wantWholeLines(t, out, "zsh:1: read-only variable: functrace")
	})

	t.Run("the type word says so", func(t *testing.T) {
		out, st := runZsh(t, dir, `print -r -- "${(t)functrace}"`)
		want := "array-readonly-hide-hideval-special\n"
		if out != want || st != 0 {
			t.Errorf("${(t)functrace} = %q (status %d), want %q", out, st, want)
		}
	})
}

// TestFunctraceNamesTheShellTwoWaysAtTheTopLevelOfMinusC is the one place the
// two halves of "where am I" come apart, and it is measured rather than
// reasoned.
//
// Under `-c` there is no file for a top-level call to be located in, and zsh
// 5.9.2 fills the gap differently depending on what was entered: a
// **function** is located at `$0`, a **sourced file** at the shell's own
// fixed name. Measured through a symlink called `./myzsh`, which is what
// makes the row discriminating — with the binary invoked by its real path
// the two strings are both `zsh` and the pair looks like one rule:
//
//	./myzsh -f -c 'f(){ … }<newline>f'        ./myzsh:2
//	./myzsh -f -c '<newline>. ./src.zsh'      zsh:2
//
// So the runner here is named `./myzsh` for the same reason. A `$0` used for
// both would pass the first row and write `./myzsh` for the second; the
// dialect's own name used for both would pass the second and lose the first.
func TestFunctraceNamesTheShellTwoWaysAtTheTopLevelOfMinusC(t *testing.T) {
	dir := t.TempDir()
	body := "\n\nh(){ " + trace + " }\nh\n"
	if err := os.WriteFile(filepath.Join(dir, "src.zsh"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("a function is located at $0", func(t *testing.T) {
		out, st := runZshNamed(t, dir, "./myzsh", "f(){ "+trace+" }\nf")
		want := "1 [./myzsh:2]\n"
		if out != want || st != 0 {
			t.Errorf("functrace = %q (status %d), want %q", out, st, want)
		}
	})

	t.Run("a sourced file is located at the shell's own name", func(t *testing.T) {
		out, st := runZshNamed(t, dir, "./myzsh", "\n. ./src.zsh")
		want := "2 [./src.zsh:4][zsh:2]\n"
		if out != want || st != 0 {
			t.Errorf("functrace = %q (status %d), want %q", out, st, want)
		}
	})
}

// runZshNamed runs src with the shell's `$0` set to something other than its
// own name, which is the only way to tell the two apart. See the test above.
func runZshNamed(t *testing.T, dir, name, src string) (string, int) {
	t.Helper()
	out, st, err := preset.Combined(t, dialecttest.Base{
		Dir: dir, Name: name, Vars: map[string]string{"PATH": dir},
	}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}
