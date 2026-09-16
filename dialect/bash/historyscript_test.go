// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// History expansion in a **script**, which is bash's row alone.
//
// Every expected string below is a transcript: the file was run through bash
// 5.3.20 on 2026-09-16 with the two streams captured apart, and what is
// written here is what came back. bash 3.2.57 and the same binary invoked as
// `sh` answer identically on every one of them, which is why this is not
// three tables.
//
// The one thing to know before reading any of it: bash expands each
// **physical line as it reads it**, and every row is a consequence of that.

// historyRun runs src as a script file and answers the two streams and the
// status.
func historyRun(t *testing.T, src string) (string, string, int) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "s.sh")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errs bytes.Buffer
	code := driver.MainArgs(bashShell(&out, &errs), []string{"bash", path})
	// The script's own name is in every diagnostic and is a temporary
	// directory's, so it is taken out rather than asserted.
	return out.String(), strings.ReplaceAll(errs.String(), path, "S"), code
}

// The issue's own probe, byte for byte. Both streams, because half of what the
// feature does is the echo.
func TestHistoryExpansionRunsInAScript(t *testing.T) {
	out, errs, code := historyRun(t, `set -o history
set -H
echo one two three
echo !!
echo !$
echo hello world
^hello^goodbye^
`)
	wantOut := "one two three\necho one two three\nthree\nhello world\ngoodbye world\n"
	wantErr := "echo echo one two three\necho three\necho goodbye world\n"
	if out != wantOut || errs != wantErr || code != 0 {
		t.Errorf("ran\n out %q\n err %q\n status %d\nwant\n out %q\n err %q\n status 0",
			out, errs, code, wantOut, wantErr)
	}
}

// Both options, and neither on its own. Measured: `set -H` with no list to
// index expands nothing, and a list with the expander off is a list nobody
// reads.
func TestHistoryExpansionInAScriptNeedsBothOptions(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		{"the expander alone", "set -H\necho one two three\necho !!\n"},
		{"the list alone", "set -o history\necho one two three\necho !!\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, _ := historyRun(t, c.src)
			if out != "one two three\n!!\n" || errs != "" {
				t.Errorf("ran %q with %q on stderr, want the two characters and nothing said", out, errs)
			}
		})
	}
}

// The line is read whole before any of it runs, so the line that turns the
// expander on cannot expand.
func TestTheLineThatTurnsTheExpanderOnDoesNotExpand(t *testing.T) {
	out, _, _ := historyRun(t, "set -o history\necho one two three\nset -H; echo !!\n")
	if out != "one two three\n!!\n" {
		t.Errorf("ran %q, want the reference left alone on its own line", out)
	}
}

// And either option turning off stops it again, from the next line. `set +o
// history` is the one worth measuring rather than reasoning about: it stops
// the *expander* as well as the list, so the two options are an AND held
// continuously and not a sequence — and a later `set -o history` starts both
// again over the entries the list still holds.
func TestEitherOptionTurningOffStopsExpandingInAScript(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"set +H", "echo one two three\necho !!\nset +H\necho !!\n", "one two three\necho one two three\n!!\n"},
		{"set +o history", "echo one two three\nset +o history\necho !!\n", "one two three\n!!\n"},
		{
			"and the list survives being turned off and on",
			"echo one\nset +o history\necho two\nset -o history\necho three\necho !!\nhistory\n",
			// `set +o history` is in the list because the line was recorded
			// before it ran; `echo two`, the line after it, is not.
			"one\ntwo\nthree\necho three\n    1  echo one\n    2  set +o history\n" +
				"    3  echo three\n    4  echo echo three\n    5  history\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _, _ := historyRun(t, "set -o history; set -H\n"+c.src)
			if out != c.want {
				t.Errorf("ran %q, want %q", out, c.want)
			}
		})
	}
}

// What a physical line begins inside is what decides whether it expands, and
// the state crosses the boundary. Four transcripts, one rule.
func TestAQuoteCrossesThePhysicalLineBoundary(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		// Inside double quotes a reference expands exactly as it does
		// outside one.
		{"double quotes carry and still expand", "echo \"a\n!!\nb\"\n", "a\necho one two three\nb\n"},
		// Inside single quotes nothing expands, on this line or the next.
		{"single quotes carry and protect", "echo 'a\n!!\nb'\n", "a\n!!\nb\n"},
		// And the protection ends where the quote does, part way through a
		// line the parser was still inside one at.
		{"the quote closing lets the rest expand", "echo 'a\nb' !!\n", "a\nb echo one two three\n"},
		// A here-document's body is not shell text: no expansion, with a
		// quoted delimiter or an unquoted one.
		{"an unquoted here-document body is left alone", "cat <<EOD\nx !! y\nEOD\n", "x !! y\n"},
		{"a quoted here-document body is left alone", "cat <<'EOD'\nx !! y\nEOD\n", "x !! y\n"},
		// A reference on the far side of a line continuation is expanded,
		// because the continuation line is a physical line of its own.
		{"a continuation line expands", "echo a \\\n!! b\n", "a echo one two three b\n"},
		// A substitution spanning lines is ordinary text as far as this is
		// concerned.
		{"a command substitution spanning lines expands", "x=$(echo\n!!)\necho \"[$x]\"\n", "[\none two three]\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _, _ := historyRun(t, "set -o history; set -H\necho one two three\n"+c.src)
			want := "one two three\n" + c.want
			if out != want {
				t.Errorf("ran %q, want %q", out, want)
			}
		})
	}
}

// A function's body is expanded when the body is **read**, not when the
// function is called: the definition that lands is already the expansion.
func TestAFunctionBodyIsExpandedWhenItIsRead(t *testing.T) {
	out, _, _ := historyRun(t, `set -o history; set -H
echo one two three
f() {
  echo !!
}
echo later words
f
`)
	want := "one two three\nlater words\necho one two three\n"
	if out != want {
		t.Errorf("ran %q, want %q — the body must hold what `!!` meant when it was read", out, want)
	}
}

// Only the program the shell is reading. An alias body, an `eval` string and
// a sourced file are all text the *runner* reads later, and none of them is
// expanded — measured on all three.
func TestOnlyTheProgramBeingReadIsExpanded(t *testing.T) {
	dir := t.TempDir()
	inc := filepath.Join(dir, "inc.sh")
	if err := os.WriteFile(inc, []byte("echo !!\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ name, src, want string }{
		{"an alias body", "shopt -s expand_aliases\nalias bang='echo !!'\nbang\n", "!!\n"},
		{"an eval string", "eval 'echo !!'\n", "!!\n"},
		{"a sourced file", ". " + inc + "\n", "!!\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _, _ := historyRun(t, "set -o history; set -H\necho one two three\n"+c.src)
			want := "one two three\n" + c.want
			if out != want {
				t.Errorf("ran %q, want %q", out, want)
			}
		})
	}
}

// A reference the list does not hold is a complaint naming the script and the
// line, the line does not run, and the shell goes on to the next one.
func TestAReferenceTheListDoesNotHoldDropsTheLine(t *testing.T) {
	out, errs, code := historyRun(t, "set -o history; set -H\necho before\necho !nosuch\necho after\n")
	if out != "before\nafter\n" || code != 0 {
		t.Errorf("ran %q at %d, want the line dropped and the script carrying on at 0", out, code)
	}
	if errs != "S: line 3: !nosuch: event not found\n" {
		t.Errorf("said %q, want the script and the line named", errs)
	}
}

// And the dropped line is dropped **before the parser sees it**, which is
// visible in every line number after it: measured, a second bad reference on
// the file's line 5 is reported at `line 4`, a `$LINENO` on the file's line 4
// reads 3, and a syntax error on the file's line 5 is reported at line 4.
//
// Not a rounding error to be tidied away — it is what bash does, and a shell
// numbering from the file would answer a different line for every diagnostic
// after the first dropped reference.
func TestEverythingAfterADroppedLineIsNumberedWithoutIt(t *testing.T) {
	out, errs, _ := historyRun(t, "set -o history; set -H\necho before\necho !nosuch\necho $LINENO\necho !nosuch2\necho end\n")
	if out != "before\n3\nend\n" {
		t.Errorf("ran %q, want $LINENO reading 3 on the file's line 4", out)
	}
	want := "S: line 3: !nosuch: event not found\nS: line 4: !nosuch2: event not found\n"
	if errs != want {
		t.Errorf("said %q, want %q", errs, want)
	}
	// The same shift reaches a parse failure after it.
	_, errs, code := historyRun(t, "set -o history; set -H\necho before\necho !nosuch\necho after\nfor\n")
	if !strings.Contains(errs, "line 4: syntax error") || code == 0 {
		t.Errorf("said %q at %d, want the failure on the file's line 5 reported at line 4", errs, code)
	}
}

// The list a script builds, read back by `history`. Every column of this is
// measured, and three of them could not have been seen at a prompt.
func TestTheListAScriptBuilds(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		// The command is in the list before it runs, so `history` lists
		// itself — and the line that turned the list on is not in it.
		{
			"a command is in the list before it runs",
			"set -o history\necho one\nhistory\n",
			"one\n    1  echo one\n    2  history\n",
		},
		// A multi-line command is one entry, its newlines written as the
		// separators the text can take.
		{
			"a compound command is one entry",
			"set -o history\nif true\nthen\n  echo hi\nfi\nhistory\n",
			"hi\n    1  if true; then   echo hi; fi\n    2  history\n",
		},
		{
			"a loop is one entry",
			"set -o history\nfor i in 1 2\ndo\necho $i\ndone\nhistory\n",
			"1\n2\n    1  for i in 1 2; do echo $i; done\n    2  history\n",
		},
		// Except where a `;` would be body text rather than a separator.
		{
			"a here-document keeps its newlines",
			"set -o history\ncat <<EOD\nbody\nEOD\nhistory\n",
			"body\n    1  cat <<EOD\nbody\nEOD\n\n    2  history\n",
		},
		// A comment line is an entry of its own; a blank line is not an
		// entry at all.
		{
			"a comment is an entry and a blank line is not",
			"set -o history\n# a comment\n\necho a\nhistory\n",
			"a\n    1  # a comment\n    2  echo a\n    3  history\n",
		},
		// A line of *blanks* is an entry, which is the discriminator for
		// where the rule lives: the test is emptiness and not blankness, and
		// a shell trimming first would drop these two.
		{
			"a line of blanks is an entry",
			"set -o history\n   \n\t\necho a\nhistory\n",
			"a\n    1     \n    2  \t\n    3  echo a\n    4  history\n",
		},
		// And a blank line *inside* a command is kept, with the separators
		// that say the semicolon was not doubled over it.
		{
			"a blank line inside a command",
			"set -o history\nif true\n\nthen\necho hi\nfi\nhistory\n",
			"hi\n    1  if true;  then echo hi; fi\n    2  history\n",
		},
		// What goes in is the **expanded** text, so each reference resolves
		// against what the one before it produced.
		{
			"the expanded text is what is stored",
			"set -o history; set -H\necho AAA\necho !!\nhistory\n",
			"AAA\necho AAA\n    1  echo AAA\n    2  echo echo AAA\n    3  history\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _, _ := historyRun(t, c.src)
			if out != c.want {
				t.Errorf("ran %q, want %q", out, c.want)
			}
		})
	}
}

// The list is the one `history` itself keeps, which is what makes a planted
// entry reachable. And because a command is in the list before it runs, `!!`
// after `history -s` names what was planted rather than the planting.
func TestAPlantedEntryIsReachableFromAReference(t *testing.T) {
	out, _, _ := historyRun(t, "set -o history; set -H\nhistory -s \"echo planted\"\necho !!\n")
	if out != "echo planted\n" {
		t.Errorf("ran %q, want the planted entry recalled", out)
	}
}

// `history -p` expands against that same list, and does it with the expander
// off — the letter decides whether the shell expands what it *reads*.
func TestHistoryDashPExpandsAgainstTheList(t *testing.T) {
	out, _, code := historyRun(t, "set -o history\necho one two three\nhistory -p '!!' '!$' nothing\n")
	if out != "one two three\necho one two three\nthree\nnothing\n" || code != 0 {
		t.Errorf("ran %q at %d, want the three operands expanded", out, code)
	}
}

// And a reference an empty list cannot answer is the failure bash reports,
// with none of the operands written.
func TestHistoryDashPWithNothingToExpandAgainst(t *testing.T) {
	out, errs, code := historyRun(t, "history -p '!!'\n")
	if out != "" || code != 1 {
		t.Errorf("wrote %q at %d, want nothing at 1", out, code)
	}
	if !strings.Contains(errs, "history: !!: history expansion failed") {
		t.Errorf("said %q, want the expansion-failed sentence", errs)
	}
}

// A reference with no word designator is the entry's own text, spacing and
// all; one with a designator is words joined by single spaces. Nothing at a
// prompt could tell the two apart.
func TestAReferenceWithNoDesignatorIsVerbatim(t *testing.T) {
	// Asserted on the **echo**, because `echo` itself collapses the runs of
	// blanks and stdout could not tell the two apart. That is the instrument
	// question this row turns on: the first version of this test read stdout
	// and passed whatever the expansion did.
	out, errs, _ := historyRun(t, "set -o history; set -H\necho   spaced    words\necho !!\necho !!:*\n")
	wantErr := "echo echo   spaced    words\necho echo spaced words\n"
	if errs != wantErr {
		t.Errorf("echoed %q, want %q", errs, wantErr)
	}
	if out != "spaced words\necho spaced words\necho spaced words\n" {
		t.Errorf("ran %q", out)
	}
}

// `histchars` moves the characters in a script as it does at a prompt.
func TestHistcharsMovesTheCharactersInAScript(t *testing.T) {
	out, _, _ := historyRun(t, "set -o history; set -H\nhistchars='@^#'\necho one two three\necho @@\n")
	if out != "one two three\necho one two three\n" {
		t.Errorf("ran %q, want the moved character taken as the event", out)
	}
}

// The other two non-interactive routes read the same way, which is measured:
// `-c` and a program on standard input both expand.
func TestTheOtherScriptRoutesExpandToo(t *testing.T) {
	src := "set -o history\nset -H\necho one two three\necho !!\n"
	for _, c := range []struct {
		name string
		argv []string
		in   string
	}{
		{"a command string", []string{"bash", "-c", src}, ""},
		{"a program on standard input", []string{"bash"}, src},
	} {
		t.Run(c.name, func(t *testing.T) {
			var out, errs bytes.Buffer
			sh := bashShell(&out, &errs)
			if c.in != "" {
				sh.Stdin = strings.NewReader(c.in)
			}
			if code := driver.MainArgs(sh, c.argv); code != 0 {
				t.Fatalf("status %d (stderr %q)", code, errs.String())
			}
			if out.String() != "one two three\necho one two three\n" {
				t.Errorf("ran %q, want the reference expanded", out.String())
			}
			if errs.String() != "echo echo one two three\n" {
				t.Errorf("said %q, want the expanded line echoed", errs.String())
			}
		})
	}
}

// `set -v` writes back the **expanded** line, which is measured and is what
// says the gate replaces the source text rather than adding to it: under
// `set -o history; set -H; set -v`, the file's `echo !!` is echoed as `echo
// echo one two three`, and the history expander then echoes the same text
// again on its own account.
func TestVerboseEchoesTheExpandedLine(t *testing.T) {
	out, errs, _ := historyRun(t, "set -o history; set -H\nset -v\necho one two three\necho !!\n")
	wantErr := "echo one two three\necho echo one two three\necho echo one two three\n"
	if errs != wantErr {
		t.Errorf("echoed %q, want %q", errs, wantErr)
	}
	if out != "one two three\necho one two three\n" {
		t.Errorf("ran %q", out)
	}
}

// A script that never asks for the feature is untouched — the `!` characters
// reach the parser as they always did, and the three places a `!` is
// something else stay what they are.
func TestAScriptWithoutTheOptionsIsUnchanged(t *testing.T) {
	out, errs, code := historyRun(t, "echo a!b\n[[ ! -e /nonesuch ]] && echo negated\necho $((3 != 4))\n")
	if out != "a!b\nnegated\n1\n" || errs != "" || code != 0 {
		t.Errorf("ran %q / %q at %d, want the three lines untouched", out, errs, code)
	}
}
