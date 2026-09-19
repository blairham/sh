// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// #3156: `--help` was `no such option: help` at 1, because the front end hands
// any unmatched `--word` to the option table and this is not an option name.
// Real zsh 5.9.2 writes a usage block on **standard output** at **0**,
// measured 2026-09-18 under `env -i PATH=/usr/bin:/bin LC_ALL=C` with standard
// input on /dev/null.
//
// Against the *binary's* shell value rather than the dialect's, which is the
// half a package test cannot reach: the option has to arrive through the front
// end a shebang, `chsh` and an editor's shell setting actually name. Same
// reasoning as the `--emulate` suite beside this one.

func runZsh(t *testing.T, argv ...string) (string, string, int) {
	t.Helper()
	var out, errs strings.Builder
	sh := scratchShell(t)
	sh.Stdout, sh.Stderr = &out, &errs
	// The run first, then the three values. Returning them in one expression
	// read the two builders before MainArgs had written anything — every row
	// came back empty and every status was the shell's, which is a suite that
	// reports a working shell as broken and an empty one as fine.
	code := driver.MainArgs(sh, append([]string{"zsh"}, argv...))
	return out.String(), errs.String(), code
}

func TestTheHelpOptionWritesTheBlockOnStandardOutput(t *testing.T) {
	out, errs, code := runZsh(t, "--help")
	if code != 0 {
		t.Fatalf("status %d, want 0 — out %q, stderr %q", code, out, errs)
	}
	if errs != "" {
		t.Errorf("stderr %q, want nothing — this is an answer and not a complaint", errs)
	}
	if !strings.HasPrefix(out, "Usage: zsh [<options>] [<argument> ...]\n") {
		t.Errorf("stdout opens %q, want the usage line naming the shell",
			strings.SplitN(out, "\n", 2)[0])
	}
	for _, heading := range []string{"Special options:", "Named options:", "Option aliases:", "Option letters:"} {
		if !strings.Contains(out, "\n"+heading+"\n") {
			t.Errorf("stdout has no %q section", heading)
		}
	}
	// It is a listing and not a line: a block that lost its tables would
	// still pass every assertion above.
	if n := strings.Count(out, "\n"); n < 200 {
		t.Errorf("%d lines, want the whole roster — the tables are not reaching the block", n)
	}
}

// **Nothing in the block is advertised without being run.** The four spellings
// the prose claims and the five special options the first section lists are
// each exercised here, because a usage block that names an option the front
// end refuses is worse than no block: it is a wrong answer a reader acts on.
//
// This is what kept `-b` out of the list, and what now keeps it in. The
// reference takes it as the end of option processing — `zsh -b -c 'echo ran'`
// there reads `-c` as the script name and answers `can't open input file: -c`
// — and this front end refused the letter outright until #3754 gave it
// Semantics.EndOfOptionsInvocationLetter. It was in the first draft of the
// block, this test is what took it out, and the row below is what earns it
// back.
func TestEverySpellingTheHelpBlockAdvertisesWorks(t *testing.T) {
	// The four spellings the prose claims, each read back through the option
	// it names rather than through an exit status: a front end that took the
	// word and moved nothing would pass a status check.
	probe := `[[ -o globdots ]] && echo ON || echo off`
	for _, c := range []struct {
		name string
		argv []string
		want string
	}{
		{"--NAME turns it on", []string{"--globdots", "-c", probe}, "ON\n"},
		{"--no-NAME turns it off", []string{"--no-globdots", "-c", probe}, "off\n"},
		{"-o NAME turns it on", []string{"-o", "globdots", "-c", probe}, "ON\n"},
		{"+o NAME turns it off", []string{"+o", "globdots", "-c", probe}, "off\n"},
		// The negative spelling the letter rows name, and the positive name
		// that puts it back. `--no-noglob' is **not** a spelling — measured,
		// the reference refuses it too (`no such option: no_noglob`) — which
		// is why the prose does not claim it. The first draft did, and this
		// row is what took the claim out.
		{"a negative spelling turns it off", []string{
			"--noglob", "-c",
			`[[ -o glob ]] && echo ON || echo off`,
		}, "off\n"},
		{"so does the `no-' spelling of the positive name", []string{
			"--no-glob", "-c",
			`[[ -o glob ]] && echo ON || echo off`,
		}, "off\n"},
		{"and the positive name turns it back on", []string{
			"--glob", "-c",
			`[[ -o glob ]] && echo ON || echo off`,
		}, "ON\n"},
		// The special options, each doing the thing the block says it does.
		{"-c runs the argument", []string{"-c", "echo ran"}, "ran\n"},
		{"--emulate starts in the mode", []string{"--emulate", "ksh", "-c", "emulate"}, "ksh\n"},
		// `-b` stops the reading at the end of its own word, so the letters
		// welded behind it are still read and the command string still runs.
		// That the *next* word is an operand is the other half and is
		// TestTheEndOfOptionsLetterIsReadHere, which cannot live in this
		// table: every row here exits 0 and that half exits 127.
		{"-b is read as a letter", []string{"-bc", "echo ran"}, "ran\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, code := runZsh(t, c.argv...)
			if code != 0 || errs != "" {
				t.Fatalf("status %d stderr %q, want the spelling taken", code, errs)
			}
			if out != c.want {
				t.Errorf("stdout %q, want %q", out, c.want)
			}
		})
	}
	// `--version` and `--help` are the other two rows, and each answers on
	// standard output at 0.
	for _, word := range []string{"--version", "--help"} {
		out, errs, code := runZsh(t, word)
		if code != 0 || errs != "" || out == "" {
			t.Errorf("%s: status %d stderr %q out %q, want an answer on stdout at 0",
				word, code, errs, out)
		}
	}
	// **And the set is read off the block rather than written out here**, so a
	// row added to the Special options section fails this test until somebody
	// runs it. A hand-kept list would go stale in the direction that matters:
	// the block would advertise something nothing had exercised.
	exercised := map[string]bool{
		"--help": true, "--version": true, "--emulate": true,
		"-c": true, "-o": true, "+o": true, "-b": true,
	}
	for _, row := range specialOptionRows(t) {
		word := strings.Fields(row)[0]
		if !exercised[word] {
			t.Errorf("the block advertises %q and nothing above runs it", word)
		}
	}

	// And the control that says the rows above are about these words rather
	// than about a front end that takes anything: a word the block does not
	// advertise is still refused.
	_, errs, code := runZsh(t, "--nosuchoptionname", "-c", ":")
	if code == 0 || !strings.Contains(errs, "no such option") {
		t.Errorf("an unadvertised word: status %d stderr %q, want it refused", code, errs)
	}
}

// Every name the block lists is a name the shell takes, which is the guard
// that the *roster* half of the block is true and not only generated. Run
// over all of them, since the whole argument for generating it is that the
// roster moves.
func TestEveryNameTheHelpBlockListsIsAnOptionTheShellTakes(t *testing.T) {
	out, _, code := runZsh(t, "--help")
	if code != 0 {
		t.Fatalf("--help status %d", code)
	}
	_, rest, ok := strings.Cut(out, "\nNamed options:\n")
	if !ok {
		t.Fatal("no named-options section")
	}
	body, _, _ := strings.Cut(rest, "\n\n")
	names := strings.Fields(body)
	if len(names) < 100 {
		t.Fatalf("%d names, want the whole roster", len(names))
	}
	for _, name := range names {
		// Status and the refusal sentence, not an empty stderr: `--verbose`
		// and `--xtrace` do their job on that stream, and a suite that
		// demanded silence would report the two options that work loudest as
		// the two that are broken.
		_, errs, code := runZsh(t, name, "-c", ":")
		if code != 0 || strings.Contains(errs, "no such option") {
			t.Errorf("%s: status %d stderr %q — the block advertises an option the front end refuses",
				name, code, errs)
		}
	}
}

// specialOptionRows is the first section of the block, read back off the
// shell's own answer rather than off the source that built it.
func specialOptionRows(t *testing.T) []string {
	t.Helper()
	out, _, code := runZsh(t, "--help")
	if code != 0 {
		t.Fatalf("--help status %d", code)
	}
	_, rest, ok := strings.Cut(out, "\nSpecial options:\n")
	if !ok {
		t.Fatal("no special-options section")
	}
	body, _, _ := strings.Cut(rest, "\n\n")
	var rows []string
	for _, line := range strings.Split(body, "\n") {
		if row := strings.TrimSpace(line); row != "" {
			rows = append(rows, row)
		}
	}
	if len(rows) == 0 {
		t.Fatal("the special-options section is empty")
	}
	return rows
}

// `-b` ends the option reading, so the word after the one it stands in is an
// operand — which is why the command-string letter after it becomes a path
// the shell cannot open.
//
// Its own test rather than a row in the table above, because every row there
// exits 0 on standard output and this is a refusal at 127. Measured 2026-09-19
// on zsh 5.9.2 under `env -i PATH=/usr/bin:/bin LC_ALL=C` with standard input
// on the null device: `zsh -f -b -c 'echo ran'` is `can't open input file: -c`
// at 127, and `zsh -f -c 'set -b; echo ran'` is `set: bad option: -b` — the
// control that says the letter is the invocation's and not an option with a
// state. See interp.Semantics.EndOfOptionsInvocationLetter for the panel.
func TestTheEndOfOptionsLetterIsReadHere(t *testing.T) {
	out, errs, code := runZsh(t, "-b", "-c", "echo ran")
	if code != 127 || !strings.Contains(errs, "-c") || out != "" {
		t.Errorf("`-b -c 'echo ran'`: status %d out %q stderr %q, "+
			"want the word after it read as a file at 127", code, out, errs)
	}
	// And the same letter at `set` is still refused, which is what keeps it
	// out of the option table.
	_, errs, code = runZsh(t, "-c", "set -b; echo ran")
	if code == 0 || !strings.Contains(errs, "-b") {
		t.Errorf("`set -b`: status %d stderr %q, want it refused by name", code, errs)
	}
}
