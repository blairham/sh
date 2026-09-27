// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"syscall"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The command a killed-command notice writes back is written the way this
// shell writes a program back.
//
// Every route into the notice used to carry a *simple* command — an external
// command is one line however it is printed — so the arrangement was never a
// question and the tree was walked with none. A foreground subshell that a
// signal ended (#4649) put a compound command in it, and the two partings
// showed up at once: a printer keeping the source's own line breaks wrote the
// notice over two lines where the shell writes one.
//
// **The obvious fix is wrong, and the second row is here to say so.** It is
// not "the notice is one line": a compound body keeps newlines and puts them
// in places of its own. Both rows were measured 2026-09-26 on bash 5.3.20,
// `--norc --noprofile` over a script file, `go version -m` → *not a Go
// executable*, and both are byte-identical to what the arrangement below
// produces.
//
//	( echo one                      ( echo one; /bin/sh -c '…' )
//	  /bin/sh -c '…' )
//
//	( for i in 1 2; do              ( for i in 1 2;
//	    echo "$i"                   do
//	  done                              echo "$i";
//	  /bin/sh -c '…' )              done; /bin/sh -c '…' )
//
// So a mutant that collapses every newline passes the first row and fails the
// second, which is the whole reason the second row is here.
//
// The arrangement names no shell: it is the same [syntax.Layout] the shell
// writes a whole program back in, which is what a dialect that has one hands
// the runner. A dialect that has none is unmoved, because the zero value
// keeps the source's own lines.
func killedNoticeLayout() syntax.Layout {
	return syntax.Layout{
		Lines:                                   true,
		Separator:                               ";",
		KeywordTerminator:                       ";",
		Indent:                                  "    ",
		Nested:                                  true,
		DoAfterWordsOnItsOwnLine:                true,
		BraceOpenSuffix:                         " ",
		CaseHeaderSuffix:                        " ",
		StatementsShareALineOutsideADeclaration: true,
		// The field this caller answers for itself: the notice writes a
		// *command*, and this one is about a function's body.
		BodyIsAlwaysBraced: true,
	}
}

// killedLayoutRun is killedRun with an arrangement for the program this shell
// writes back.
func killedLayoutRun(t *testing.T, src string) string {
	t.Helper()
	dg := Diagnostics{
		SignalDescriptions: map[syscall.Signal]string{syscall.SIGUSR1: "Boom"},
		Location:           LocationTightLine,
		// Unprefixed, so what the row is about — the command — is the whole
		// of the line and a location cannot make a mismatch look like one.
		KilledCommandNotice:           "%5[1]d %-27[2]s%[3]s",
		KilledCommandNoticeUnprefixed: true,
	}
	var errs strings.Builder
	sem := PosixSemantics()
	sem.ReportsACommandKilledBySignal = Yes
	sem.ChildInterruptEndsTheScript = No
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &dg, Name: "sh",
		Stdout: &strings.Builder{}, Stderr: &errs,
	})
	r.SetScriptListingLayout(killedNoticeLayout())
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	// The command is whatever follows the words for the signal.
	line := strings.TrimSuffix(errs.String(), "\n")
	_, cmd, ok := strings.Cut(line, "Boom")
	if !ok {
		t.Fatalf("no notice in %q", errs.String())
	}
	return strings.TrimLeft(cmd, " ")
}

const killedSubshellOfSimpleCommands = "( echo one\n  /bin/sh -c 'kill -USR1 $$' )\n"

func TestAKilledCommandSpanningLinesIsWrittenAsOneLine(t *testing.T) {
	want := "( echo one; /bin/sh -c 'kill -USR1 $$' )"
	if got := killedLayoutRun(t, killedSubshellOfSimpleCommands); got != want {
		t.Errorf("notice command =\n%q\nwant\n%q", got, want)
	}
}

const killedSubshellWithACompoundBody = "( for i in 1 2; do\n" +
	"    echo \"$i\"\n" +
	"  done\n" +
	"  /bin/sh -c 'kill -USR1 $$' )\n"

// The row a collapse-every-newline fix fails. The newlines the arrangement
// puts in are not the ones the source had, and there are fewer of them.
func TestAKilledCompoundCommandKeepsTheNewlinesTheArrangementGivesIt(t *testing.T) {
	want := "( for i in 1 2;\ndo\n    echo \"$i\";\ndone; /bin/sh -c 'kill -USR1 $$' )"
	if got := killedLayoutRun(t, killedSubshellWithACompoundBody); got != want {
		t.Errorf("notice command =\n%q\nwant\n%q", got, want)
	}
}

// And the control: a shell with no arrangement of its own writes what it
// always wrote. The zero [syntax.Layout] keeps the source's own line
// structure, so this is the row that says the change is the dialect's to ask
// for rather than a new rule in the core.
func TestWithNoArrangementTheCommandKeepsTheSourcesLines(t *testing.T) {
	got, _ := killedRun(t, killedSubshellOfSimpleCommands, Diagnostics{
		KilledCommandNotice:           "%5[1]d %-27[2]s%[3]s",
		KilledCommandNoticeUnprefixed: true,
	}, Yes)
	if !strings.Contains(got, "( echo one\n/bin/sh -c 'kill -USR1 $$' )") {
		t.Errorf("notice = %q, want the source's own two lines", got)
	}
}
