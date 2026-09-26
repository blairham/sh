// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The `bad pattern` complaint itself, rather than which patterns earn one.
// #4630 and #4645 settled the set; these are the two things the *sentence*
// was getting wrong.
//
// Measured against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m` says *not a Go executable* —
// run `-f` over a script file, 2026-09-26.

// A word this shell will not compile as a pattern, standing as a redirection
// target, earned **two** identical complaints where the reference writes one
// (#4647, #4670 — the same finding filed twice).
//
// The cause is not a retry: a redirection target is read in more than one
// view, and each view globs, so every pattern in one reaches the refusal
// twice. The miss beside it has been asking whether the shell is already
// unwinding since it was found writing `no matches found` twice; the refusal
// was not asking, and [Runner.givingUpAlready] is now the one question both
// of them put.
//
// **The counting control is the last row**, and without it this whole table
// would pass for a shell that had lost the complaint's second copy *and* for
// one that had lost the ability to write two complaints at all. Two `eval`s,
// because an `eval` reports a bad pattern and lets the script carry on —
// measured, the reference writes both lines.
func TestABadPatternInARedirectionTargetIsReportedOnce(t *testing.T) {
	for _, c := range []struct {
		name, src string
		want      int
		status    int
	}{
		{"a redirection target", `: > [a && print -r -- OK`, 1, 1},
		{"an appending target", `: >> [a && print -r -- OK`, 1, 1},
		{"a reading target", `: < [a && print -r -- OK`, 1, 1},
		{
			// The command-word route, which was right throughout and is
			// what says the doubling is the redirection's and not the
			// refusal's.
			"a command word", `print -r -- [a`, 1, 1,
		},
		{
			// The neighboring complaint on the same route. It has had the
			// guard all along, so a change that broke it would show here
			// rather than in a file nobody reads.
			"a miss in a redirection target", `: > zz* && print -r -- OK`, 1, 1,
		},
		{
			// The positive control. A shell that could only ever write one
			// complaint passes every row above and fails this one.
			"two complaints are two lines",
			"eval 'print -r -- [a'\neval 'print -r -- [b'", 2, 1,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := badPatternDir(t)
			out, st, errs := runZshSplit(t, dir, "setopt badpattern\n"+c.src)
			if out != "" {
				t.Errorf("stdout = %q, want nothing", out)
			}
			if st != c.status {
				t.Errorf("status = %d, want %d", st, c.status)
			}
			if got := len(strings.Split(strings.TrimSuffix(errs, "\n"), "\n")); got != c.want {
				t.Errorf("stderr held %d lines, want %d: %q", got, c.want, errs)
			}
		})
	}
}

// The refusal is still a refusal: the target is not opened and no file is
// made. Asserted apart from the count above, because a shell that stopped
// complaining altogether would satisfy a count of one.
func TestARefusedRedirectionTargetNamesNoFile(t *testing.T) {
	dir := t.TempDir()
	if _, st, errs := runZshSplit(t, dir, "setopt badpattern\n: > [a\n"); st != 1 {
		t.Errorf("status = %d, want 1 (stderr %q)", st, errs)
	}
	names, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Errorf("the refused redirection made %d file(s), want none", len(names))
	}
	// The positive control for the row above: the same redirection with a
	// target that compiles does make its file, so the emptiness is the
	// refusal and not a shell that cannot write.
	if _, st, errs := runZshSplit(t, dir, "setopt badpattern\n: > plain\n"); st != 0 {
		t.Errorf("control status = %d, want 0 (stderr %q)", st, errs)
	}
	if _, err := os.Stat(filepath.Join(dir, "plain")); err != nil {
		t.Errorf("the control redirection made no file: %v", err)
	}
}

// A control character in the offending word is **rendered** rather than sent
// to the terminal (#4669). A word carrying a newline produced a complaint two
// lines long here and one line long there, so a script reading stderr saw a
// different shape.
//
// The rows are the reference's, one byte at a time, from a script file. Two
// of the C0 characters have a letter and the rest are caret notation; 0x7f is
// `^?` and the 0x80-0x9f range is `\M-` in front of that same rule over the
// low seven bits.
//
// **The last two rows are the controls**, and they are what say this is a
// rendering rather than a mangling: an ordinary space and a multibyte
// character are written through untouched.
//
// The high half above 0x9f is left out deliberately and it is not an
// oversight: it is the one place the two locales disagree — `LC_ALL=C` writes
// 0xa0, 0xc1 and 0xe9 through as bytes and a UTF-8 locale writes `\M- `,
// `\M-A` and `\M-i` — and the same split is already on the near-text route,
// whose rows were measured under `LC_ALL=C`. See
// Diagnostics.NearTextEscapesControlCharacters.
func TestABadPatternComplaintRendersAControlCharacter(t *testing.T) {
	for _, c := range []struct {
		name, escape, want string
	}{
		{"a tab", `\t`, `bad pattern: x[a\tb`},
		{"a newline", `\n`, `bad pattern: x[a\nb`},
		{"a bell", `\a`, "bad pattern: x[a^Gb"},
		{"a backspace", `\b`, "bad pattern: x[a^Hb"},
		{"a vertical tab", `\v`, "bad pattern: x[a^Kb"},
		{"a form feed", `\f`, "bad pattern: x[a^Lb"},
		{"a carriage return", `\r`, "bad pattern: x[a^Mb"},
		{"an escape", `\e`, "bad pattern: x[a^[b"},
		{"the first control character", `\x01`, "bad pattern: x[a^Ab"},
		{"delete", `\x7f`, "bad pattern: x[a^?b"},
		{"the meta of the first control character", `\x80`, `bad pattern: x[a\M-^@b`},
		{"the last meta control character", `\x9f`, `bad pattern: x[a\M-^_b`},
		{"a space is written through", ` `, "bad pattern: x[a b"},
		{"a multibyte character is written through", `é`, "bad pattern: x[aéb"},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := badPatternDir(t)
			src := "setopt badpattern\nprint -r -- x[a$'" + c.escape + "'b\n"
			out, st, errs := runZshSplit(t, dir, src)
			if out != "" {
				t.Errorf("stdout = %q, want nothing", out)
			}
			if st != 1 {
				t.Errorf("status = %d, want 1", st)
			}
			if !strings.Contains(errs, c.want) {
				t.Errorf("stderr = %q, want %q in it", errs, c.want)
			}
			// The property under the rows: whatever the word held, the
			// complaint is one line. A row asserting only the text above
			// would pass for a rendering that had kept the byte *and*
			// written the escape beside it.
			if got := strings.Count(errs, "\n"); got != 1 {
				t.Errorf("stderr held %d newlines, want 1: %q", got, errs)
			}
		})
	}
}
