// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `BAD_PATTERN` says whether a word this shell will not compile as a pattern
// is an error or an ordinary word. It was accepted and then ignored until
// #4630: `unsetopt badpattern` succeeded, `[[ -o badpattern ]]` and the
// `setopt` listing reported it back faithfully, and a malformed glob went on
// being refused in either state.
//
// **Every case below is run in both states**, which is the whole of why this
// file is shaped the way it is. The control row — the option *on* — already
// agreed before the fix, so a grid that only ever asked the question one way
// reported the option fully implemented.
//
// Measured against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m` says *not a Go executable* —
// run `-f`, 2026-09-26.

// badPatternDir builds the fixture every case here runs in:
//
//	[a      the literal name the spared word would match, were it a pattern
//	xa      what `*[a` would match if the `[` became an ordinary character
//	x[a     likewise, and the one that says it does not
//
// The second and third files are what make "left unchanged" separable from
// "the `[` is a literal": a shell doing the latter answers `*[a` with `x[a`,
// and this shell — like the reference — answers it with `*[a`.
func badPatternDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"[a", "xa", "x[a"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// The grid, in both states. `on` is what the shell has always done and is the
// control; `off` is the half #4630 is about.
//
// The surfaces are deliberately mixed, because **they are not all the same
// question** and that is the finding this file exists to pin: filename
// generation moves with the option and a `case` arm, a `[[ ]]` operand and a
// parameter expansion's pattern do not.
func TestBadPatternSparesAGlobAndNothingElse(t *testing.T) {
	for _, c := range []struct {
		name, src        string
		on, off          string
		onStatus, offSt  int
		onErr, offErrStr string
	}{
		{
			// The issue's own reduction, and the line `E01options.ztst`
			// stopped on.
			name: "a bare malformed glob",
			src:  `print -r -- [a`,
			on:   "", onStatus: 1, onErr: "bad pattern: [a",
			off: "[a\n", offSt: 0,
		},
		{
			// The row that says the spared word is handed back **whole**
			// rather than read with the `[` as an ordinary character. The
			// file `x[a` is there to be matched and is not matched: a
			// literal reading would answer `x[a` here.
			name: "a malformed glob with a live star in front of it",
			src:  `print -r -- *[a`,
			on:   "", onStatus: 1, onErr: "bad pattern: *[a",
			off: "*[a\n", offSt: 0,
		},
		{
			// The bracket need not open the word — the rule is the whole
			// field rather than where the bracket sits in it.
			name: "a bracket in the middle of the word",
			src:  `print -r -- a[b`,
			on:   "", onStatus: 1, onErr: "bad pattern: a[b",
			off: "a[b\n", offSt: 0,
		},
		{
			// A negation that nothing closes is the same question.
			name: "a negated bracket that nothing closes",
			src:  `print -r -- [!`,
			on:   "", onStatus: 1, onErr: "bad pattern: [!",
			off: "[!\n", offSt: 0,
		},
		{
			// **The field that is exactly `[` is not this question in
			// either state**, and it is what keeps `[ a = a ]` working. It
			// holds the bracket fixed and moves nothing, so it is the row
			// that says the option is not keyed on "a word with a bracket
			// in it".
			name: "a field that is exactly a bracket",
			src:  `print -r -- [`,
			on:   "[\n", onStatus: 0,
			off: "[\n", offSt: 0,
		},
		{
			// The same noun from the other side: a pattern that compiles is
			// untouched by the option, so a glob that matches still
			// matches with the refusal withheld.
			name: "a well-formed pattern is the control",
			src:  `print -r -- x*`,
			on:   "x[a xa\n", onStatus: 0,
			off: "x[a xa\n", offSt: 0,
		},
		{
			// It is per **word**, not per command: the first globs and the
			// second is left alone, in one expansion.
			name: "one good word and one bad one",
			src:  `print -r -- x* [a`,
			on:   "", onStatus: 1, onErr: "bad pattern: [a",
			off: "x[a xa [a\n", offSt: 0,
		},
		{
			// A redirection's target is filename generation too.
			name: "a redirection target",
			src:  `: > [a && print -r -- OK`,
			on:   "", onStatus: 1, onErr: "bad pattern: [a",
			off: "OK\n", offSt: 0,
		},
		{
			// An expansion result made live by `${~…}` is the same
			// question arrived at from the other side, and it is one gate
			// rather than two: resultReadsAsPattern only decides which axis
			// gets asked, so the refusal it would have led to is the one in
			// glob.go. Gating it there as well is behavior-neutral —
			// measured by mutation, removing that second gate changes no
			// row — so there is one read site and this row exercises it.
			name: "an expansion result read as a pattern",
			src:  `L="[a"; print -r -- ${~L}`,
			on:   "", onStatus: 1, onErr: "bad pattern: [a",
			off: "[a\n", offSt: 0,
		},
		{
			// **The four rows below are the other half of the grid and they
			// do not move.** A `case` arm is not filename generation, and
			// measured it is refused with the option off exactly as with it
			// on. This is the pair that pins the noun: the text is `[a` in
			// this row and in the first row of this table, and only one of
			// them moves.
			name: "a case arm is refused in both states",
			src:  `case "[a" in ([a) print M;; (*) print O;; esac`,
			on:   "", onStatus: 1, onErr: "bad pattern: [a",
			off: "", offSt: 1, offErrStr: "bad pattern: [a",
		},
		{
			name: "a condition's pattern operand is refused in both states",
			src:  `[[ "[a" == [a ]] && print M || print N`,
			on:   "", onStatus: 2, onErr: "bad pattern: [a",
			off: "", offSt: 2, offErrStr: "bad pattern: [a",
		},
		{
			// An element filter's pattern, which is this dialect's own
			// surface and reaches the matcher rather than the walk.
			name: "an element filter is refused in both states",
			src:  `v="[a"; print -r -- "X${v:#[a}Y"`,
			on:   "", onStatus: 1, onErr: "bad pattern: [a",
			off: "", offSt: 1, offErrStr: "bad pattern: [a",
		},
		{
			// And the option reports itself, which it always did — the
			// state was never the thing that was wrong.
			name: "the option reports its own state",
			src:  `[[ -o badpattern ]] && print ON || print OFF`,
			on:   "ON\n", onStatus: 0,
			off: "OFF\n", offSt: 0,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, state := range []struct {
				name, setopt, want, wantErr string
				status                      int
			}{
				{"on", "setopt badpattern\n", c.on, c.onErr, c.onStatus},
				{"off", "unsetopt badpattern\n", c.off, c.offErrStr, c.offSt},
			} {
				t.Run(state.name, func(t *testing.T) {
					dir := badPatternDir(t)
					out, st, errs := runZshSplit(t, dir, state.setopt+c.src)
					if out != state.want {
						t.Errorf("stdout = %q, want %q", out, state.want)
					}
					if st != state.status {
						t.Errorf("status = %d, want %d", st, state.status)
					}
					switch {
					case state.wantErr == "" && errs != "":
						t.Errorf("stderr = %q, want nothing", errs)
					case state.wantErr != "" && !strings.Contains(errs, state.wantErr):
						t.Errorf("stderr = %q, want %q in it", errs, state.wantErr)
					}
				})
			}
		})
	}
}

// The refusal goes to **standard error** and the spared word to standard
// output, which a combined-stream helper cannot see. Asserted on its own
// because it is the property a diagnostic written to the wrong stream would
// pass every row above while breaking every script that reads the shell.
func TestABadPatternRefusalGoesToStandardError(t *testing.T) {
	dir := badPatternDir(t)
	out, st, errs := runZshSplit(t, dir, "setopt badpattern\nprint -r -- [a\n")
	if out != "" {
		t.Errorf("stdout = %q, want nothing — the complaint is not output", out)
	}
	if st != 1 {
		t.Errorf("status = %d, want 1", st)
	}
	if !strings.Contains(errs, "bad pattern: [a") {
		t.Errorf("stderr = %q, want the complaint in it", errs)
	}
}

// NOMATCH and NULL_GLOB decide what an *unmatched* glob does, and a word this
// option spared was never a glob — so neither of them has anything to say
// about it. Measured on zsh 5.9.2, `-f`, 2026-09-26: with `badpattern` off
// and either of them set, `print -r -- [a` still writes `[a` at status 0.
//
// The row that makes this evidence rather than decoration is the third: the
// same `nullglob` that leaves `[a` standing *does* delete a well-formed
// pattern that matched nothing, so the option is not simply being ignored.
func TestNoMatchAndNullGlobDoNotReachAWordBadPatternSpared(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		status          int
	}{
		{
			"nonomatch does not change the spared word",
			"unsetopt badpattern\nsetopt nonomatch\nprint -r -- [a\n",
			"[a\n", 0,
		},
		{
			"nullglob does not delete the spared word",
			"unsetopt badpattern\nsetopt nullglob\nprint -r -- [a\n",
			"[a\n", 0,
		},
		{
			// The positive control for the row above it. Without this, a
			// shell that had simply stopped reading `nullglob` would pass.
			"nullglob still deletes a well-formed miss",
			"unsetopt badpattern\nsetopt nullglob\nprint -r -- zz* END\n",
			"END\n", 0,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := badPatternDir(t)
			out, st, errs := runZshSplit(t, dir, c.src)
			if out != c.want || st != c.status {
				t.Errorf("out = %q status %d, want %q status %d", out, st, c.want, c.status)
			}
			if errs != "" {
				t.Errorf("stderr = %q, want nothing", errs)
			}
		})
	}
}

// `emulate sh` and `emulate ksh` turn the option off with nobody typing
// `setopt`, which is how most scripts that rely on it arrive. Measured on zsh
// 5.9.2, `-f`, 2026-09-26: both report `OFF` and leave `[a` standing, and
// `emulate zsh` and `emulate csh` report `ON` and refuse it.
//
// emulateoptions.go carried the right answer for this name before #4630 and
// moved nothing, because the name it moved was recorded.
func TestEmulationReachesBadPattern(t *testing.T) {
	for _, c := range []struct {
		emulate, want string
		status        int
		wantErr       bool
	}{
		{"sh", "OFF\n[a\n", 0, false},
		{"ksh", "OFF\n[a\n", 0, false},
		{"zsh", "ON\n", 1, true},
		{"csh", "ON\n", 1, true},
	} {
		t.Run(c.emulate, func(t *testing.T) {
			dir := badPatternDir(t)
			src := "emulate " + c.emulate + "\n" +
				"[[ -o badpattern ]] && print ON || print OFF\n" +
				"print -r -- [a\n"
			out, st, errs := runZshSplit(t, dir, src)
			if out != c.want {
				t.Errorf("stdout = %q, want %q", out, c.want)
			}
			if st != c.status {
				t.Errorf("status = %d, want %d", st, c.status)
			}
			if got := strings.Contains(errs, "bad pattern"); got != c.wantErr {
				t.Errorf("stderr = %q, want a complaint: %v", errs, c.wantErr)
			}
		})
	}
}

// A subshell gets the state its parent had, and a move inside one does not
// reach back out — the same bargain every other option in this table strikes,
// and the one a switch stored on the Runner could plausibly have broken by
// being shared rather than copied.
func TestBadPatternFollowsASubshell(t *testing.T) {
	dir := badPatternDir(t)
	src := "unsetopt badpattern\n" +
		"(print -r -- [a)\n" +
		"(setopt badpattern; [[ -o badpattern ]] && print INNER_ON)\n" +
		"[[ -o badpattern ]] && print OUTER_ON || print OUTER_OFF\n"
	out, st, errs := runZshSplit(t, dir, src)
	if want := "[a\nINNER_ON\nOUTER_OFF\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	if st != 0 || errs != "" {
		t.Errorf("status = %d stderr = %q, want 0 and nothing", st, errs)
	}
}
