// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A here-document body feeding a command the shell runs as a process of its
// own is expanded in that process, so what it writes does not come back.
//
// Measured 2026-09-07, `env -i PATH=/usr/bin:/bin` with a scratch HOME,
// ZDOTDIR and HISTFILE, over a script file, against bash 5.3.15, ksh93u+ and
// zsh 5.9.2 — unanimous — with dash the one shell on the other side, which is
// why the confinement is HeredocExpandsInTheCommandsProcess and not a core
// rule. The full thirteen-row table is at the top of heredocprocess.go.
func TestAHereDocumentsSideEffectStaysWithTheProgramItFed(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"an increment", "n=1\ncat <<END\n$(( n++ ))\nEND\nprintf 'after=%s' \"$n\"",
			"1\nafter=1",
		},
		{
			"a default assignment", "unset u\ncat <<END\n[${u:=zz}]\nEND\nprintf 'u=%s' \"${u-UNSET}\"",
			"[zz]\nu=UNSET",
		},
		{
			"an element of an array", "a=(1)\ncat <<END\n$(( a[0]++ ))\nEND\nprintf 'after=%s' \"${a[0]}\"",
			"1\nafter=1",
		},
		{
			// The write is visible to the rest of the body and only then
			// thrown away. `[1][1]` is what a refusal to write would have
			// produced, and it is nobody's answer.
			"twice in one body", "n=1\ncat <<END\n[$(( n++ ))][$(( n++ ))]\nEND\nprintf 'after=%s' \"$n\"",
			"[1][2]\nafter=1",
		},
		{
			// A here-string is a here-document's body on one line, and the
			// three shells confine it the same way.
			"a here-string", "n=1\ncat <<< \"$(( n++ ))\"\nprintf 'after=%s' \"$n\"",
			"1\nafter=1",
		},
		{
			// The program never runs, and the fork has already happened by
			// then in a real shell — so the write is gone even though
			// nothing was executed.
			"a program that cannot be run", "n=1\nnosuchcmd_zz <<END 2>/dev/null\n$(( n++ ))\nEND\nprintf 'after=%s' \"$n\"",
			"after=1",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runHeredoc(t, tc.src)
			if out != tc.want {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
		})
	}
}

// And the other side of the line: everything the shell runs itself leaves the
// write behind, in all four shells, because there is no other process for it
// to land in. The pair is what says the rule is about the fork rather than
// about here-documents.
func TestAHereDocumentFedToThisShellLeavesItsSideEffectBehind(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a builtin", "n=1\nread q <<END\n$(( n++ ))\nEND\nprintf 'after=%s' \"$n\""},
		{"the null builtin", "n=1\n: <<END\n$(( n++ ))\nEND\nprintf 'after=%s' \"$n\""},
		{"a function", "f() { cat; }\nn=1\nf <<END >/dev/null\n$(( n++ ))\nEND\nprintf 'after=%s' \"$n\""},
		{"a group", "n=1\n{ cat >/dev/null; } <<END\n$(( n++ ))\nEND\nprintf 'after=%s' \"$n\""},
		{"a loop", "n=1\nwhile read q; do :; done <<END\n$(( n++ ))\nEND\nprintf 'after=%s' \"$n\""},
		{"eval", "n=1\neval 'cat >/dev/null' <<END\n$(( n++ ))\nEND\nprintf 'after=%s' \"$n\""},
		{"a builtin reached through command", "n=1\ncommand : <<END\n$(( n++ ))\nEND\nprintf 'after=%s' \"$n\""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runHeredoc(t, tc.src)
			if !strings.HasSuffix(out, "after=2") {
				t.Errorf("out = %q, want it to end in after=2", out)
			}
		})
	}
}

// `command` is unwrapped, because it is a builtin that runs something else and
// the shells follow what it reaches: `command -p cat` confines the write
// exactly as a bare `cat` does.
//
// Its own test because the naive reading — argv[0] is a builtin, so the shell
// runs it — gets this one wrong, and gets it wrong silently.
func TestCommandIsUnwrappedWhenDecidingWhoseProcessAHereDocumentIsFor(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a program", "n=1\ncommand cat <<END\n$(( n++ ))\nEND\nprintf 'after=%s' \"$n\"", "1\nafter=1"},
		{"a program with -p", "n=1\ncommand -p cat <<END\n$(( n++ ))\nEND\nprintf 'after=%s' \"$n\"", "1\nafter=1"},
		{"a program after --", "n=1\ncommand -- cat <<END\n$(( n++ ))\nEND\nprintf 'after=%s' \"$n\"", "1\nafter=1"},
		// `command -v` runs nothing at all; it answers a question here.
		{"a question", "n=1\ncommand -v cat >/dev/null <<END\n$(( n++ ))\nEND\nprintf 'after=%s' \"$n\"", "after=2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runHeredoc(t, tc.src)
			if out != tc.want {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
		})
	}
}

// The dialect that lets the write escape gets to. dash expands the body in
// the shell — measured, `unset u; cat <<END` with `${u:=zz}` leaves `u=zz`
// there — and answering No is what says so rather than the answer being
// hard-coded to the majority.
func TestTheEscapingAnswerIsAvailable(t *testing.T) {
	sem := testSemantics()
	sem.HeredocExpandsInTheCommandsProcess = No
	out, _ := run(t, "n=1\ncat <<END\n$(( n++ ))\nEND\nprintf 'after=%s' \"$n\"", withSem(sem))
	if out != "1\nafter=1" && out != "1\nafter=2" {
		t.Fatalf("out = %q, want the body to be 1 either way", out)
	}
	if out != "1\nafter=2" {
		t.Errorf("out = %q, want after=2 — the answer that lets the write escape", out)
	}
}

// A body with no side effect asks nothing, so a dialect that has not answered
// can still run one. The axis is asked at the disagreement and nowhere else.
func TestAQuietHereDocumentAsksNothing(t *testing.T) {
	sem := testSemantics()
	sem.HeredocExpandsInTheCommandsProcess = Unspecified
	out, st := run(t, "v=hi\ncat <<END\n[$v]\nEND", withSem(sem))
	if out != "[hi]\n" || st != 0 {
		t.Errorf("out = %q status = %d, want the body through and 0", out, st)
	}
	// And one that does write refuses rather than guessing: the diagnostic,
	// status 2, and the program not run — the body never reaches it.
	out, st = run(t, "n=1\ncat <<END\n$(( n++ ))\nEND", withSem(sem))
	if st != 2 {
		t.Errorf("status = %d, want 2 — an unanswered axis is a refusal", st)
	}
	if !strings.Contains(out, "no dialect was chosen") {
		t.Errorf("out = %q, want the refusal named", out)
	}
	if strings.Contains(out, "1\n") {
		t.Errorf("out = %q, want the body never to have reached the program", out)
	}
}

// A body that could not be expanded costs the command and not the shell, and
// the command does not run.
//
// Unanimous, dash included: measured, `set -u; cat <<END` with an unset name
// in the body writes the diagnostic, `cat` never runs, the status is the
// dialect's fatal one and the next command in the script does run. Here the
// body's value was handed to `cat` anyway and the whole script was then
// abandoned — two wrong answers, one on top of the other.
//
// The fourth site of the boundary `.`, `eval`, a startup file and an
// interactive prompt already stand on, and the axis those split over is not
// asked at this one: there is nothing for it to disagree about.
func TestAFailedHereDocumentBodyCostsTheCommand(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"an unset name", "set -u\ncat <<END\n[$NOPE_H]\nEND\nprintf 'st=%s alive' \"$?\""},
		{"a bad substitution", "q=x\ncat <<END\n[${q@@@bad}]\nEND\nprintf 'st=%s alive' \"$?\""},
		{"a division by zero", "cat <<END\n[$(( 1/0 ))]\nEND\nprintf 'st=%s alive' \"$?\""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runHeredoc(t, tc.src)
			if !strings.Contains(out, "alive") {
				t.Errorf("out = %q, want the script to carry on", out)
			}
			// Compared against the same failure in a simple command rather
			// than against a number: which number a fatal error carries is
			// FatalErrorStatusIsOne's, and the claim here is only that this
			// failure carries it too.
			_, want := run(t, `set -u; printf "[%s]" ${NOPE_H}`, nil)
			if !strings.Contains(out, "st="+itoa(want)) {
				t.Errorf("out = %q, want st=%d — the status the same failure leaves in a simple command", out, want)
			}
			if strings.Contains(out, "[") {
				t.Errorf("out = %q, want the body never to have reached the program", out)
			}
			if st != 0 {
				t.Errorf("status = %d, want 0 — the last command is the printf", st)
			}
		})
	}
}

// The same failure fed to something the shell runs itself is *not* caught
// here, because it did not happen in another process.
//
// bash and zsh both end the script for it — measured, `set -u; read x <<END`
// with an unset name in the body prints the diagnostic and nothing after it —
// and that is what this shell does. The guard has to be about whose process
// the body is for, or it would have made every here-document survivable.
func TestAFailedHereDocumentBodyForThisShellIsStillFatal(t *testing.T) {
	out, _ := runHeredoc(t, "set -u\nread q <<END\n[$NOPE_H]\nEND\nprintf 'alive'")
	if strings.Contains(out, "alive") {
		t.Errorf("out = %q, want the script to have stopped", out)
	}
}

// runHeredoc runs a script with the here-string operator in the grammar, so
// one test file can hold both spellings.
func runHeredoc(t *testing.T, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.Herestring = true
		d.ArrayLiteral = true
	}, nil)
}

// Whose process a here-document is for is a fact about one redirection list,
// and a command reached from inside a body answers it for itself.
//
// Two here-documents on one external command, with a substitution in the
// first that runs a *builtin* with a here-document of its own: the inner
// command's answer — "this shell" — must not be the answer when the outer
// command's second body is expanded. It is not, and by two mechanisms rather
// than one: applyRedirs restores the field, and the substitution runs on a
// clone that could not write it back anyway. So this case does not distinguish
// them — see applyRedirs for the mutant that showed that — and what it does
// pin is that the nesting works at all, in a shape where getting it wrong
// would be silent.
func TestWhoseProcessAHereDocumentIsForIsPerRedirectionList(t *testing.T) {
	out, _ := runHeredoc(t, "n=1\nm=1\ncat <<A <<B\n$(( n++ ))$(read z <<C\n$(( m++ ))\nC\n)\nA\n$(( n++ ))\nB\nprintf 'n=%s m=%s' \"$n\" \"$m\"")
	if !strings.HasSuffix(out, "n=1 m=1") {
		t.Errorf("out = %q, want it to end in n=1 m=1 — both bodies confined to the program they fed", out)
	}
}
