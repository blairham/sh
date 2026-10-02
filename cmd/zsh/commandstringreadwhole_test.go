// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// **A `-c` string is read whole, so a grammar option one of its lines sets
// reaches nothing later in the same string** (#5420). The front end checked the
// string whole and then read it again a line at a time as it ran, handing each
// later line whatever grammar the line before had left behind.
//
// Measured 2026-10-02 on `/opt/homebrew/bin/zsh`, zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `-f`, each pair as `-c` and as the same two
// lines in a file. The file column is the control: the same text read as it
// runs, which is where the option does reach the next line, so a fix that
// froze the grammar everywhere fails it.
func TestAGrammarOptionTheCommandStringSetsDoesNotReachItsOwnLaterLines(t *testing.T) {
	for _, c := range []struct {
		name, src           string
		whole, file         string
		wholeErr, fileError string
	}{
		{
			"rcquotes", "setopt rcquotes\nprint -r -- 'a''b'\n",
			"ab\n", "a'b\n", "", "",
		},
		{
			"shortloops off", "unsetopt shortloops\nfor i in 1; print $i\n",
			"1\n", "", "", "parse error near `print'",
		},
		{
			"shglob", "setopt shglob\nprint @(a|b)\n",
			"", "", "no matches found: @(a|b)", "parse error near `('",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, _ := runZsh(t, "-f", "-c", c.src)
			if out != c.whole || !strings.Contains(errs, c.wholeErr) {
				t.Errorf("-c: got %q %q, want %q with %q", out, errs, c.whole, c.wholeErr)
			}
			script := filepath.Join(t.TempDir(), "s.zsh")
			if err := os.WriteFile(script, []byte(c.src), 0o600); err != nil {
				t.Fatal(err)
			}
			out, errs, _ = runZsh(t, "-f", script)
			if out != c.file || !strings.Contains(errs, c.fileError) {
				t.Errorf("a file: got %q %q, want %q with %q", out, errs, c.file, c.fileError)
			}
		})
	}
}

// **And the string is read after the startup files**, which is the other half
// of when the read happens. Measured the same day with `ZDOTDIR` at a directory
// holding only a `.zshenv`:
//
//	.zshenv                     -c                                  zsh 5.9.2
//	setopt rcquotes             unsetopt rcquotes⏎print 'a''b'     a'b
//	setopt cshjunkieloops       for i in 1 2; print $i; end         1, 2
//	echo ENV                    if; then                            ENV, then the parse error, 1
//	trap 'echo TRAP' EXIT       if; then                            the parse error, then TRAP, 1
//
// The second row is the sharpest: the text is refused in the shell's own
// grammar and accepted in the one the file left behind, so a check made before
// the file ran refused a program the reference runs.
func TestTheCommandStringIsReadAfterTheStartupFiles(t *testing.T) {
	for _, c := range []struct {
		name, env, src string
		out, err       string
		code           int
	}{
		{
			"an option the file set is the grammar", "setopt rcquotes\n",
			"unsetopt rcquotes\nprint -r -- 'a''b'\n", "a'b\n", "", 0,
		},
		{
			"a grammar the shell's own refuses", "setopt cshjunkieloops\n",
			"for i in 1 2; print $i; end\n", "1\n2\n", "", 0,
		},
		{
			"the file runs before the refusal", "echo ENV\n",
			"if; then", "ENV\n", "parse error near `then'", 1,
		},
		{
			"an exit trap the file set still fires", "trap 'echo TRAP' EXIT\n",
			"if; then", "TRAP\n", "parse error near `then'", 1,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			home := scratchHome(t)
			writeHomeFile(t, home, ".zshenv", c.env)
			out, errs, code := runZsh(t, "-c", c.src)
			if out != c.out || code != c.code {
				t.Errorf("got %q status %d (stderr %q), want %q at %d", out, code, errs, c.out, c.code)
			}
			if c.err == "" && errs != "" || !strings.Contains(errs, c.err) {
				t.Errorf("stderr %q, want %q", errs, c.err)
			}
		})
	}
}
