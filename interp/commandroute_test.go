// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Whose process a `command`-prefixed command's redirections belong to follows
// [Semantics.CommandReachesABuiltin], because that axis is what says whether
// the word reaches a builtin at all.
//
// No axis of this file's own: where `command` names an external program and
// nothing else, the command *is* a process of its own, so its redirection
// target and its here-document body are expanded there — and the write a
// target made does not come back. Where it reaches the builtin, there is no
// other process and the write stays.
//
// The tests name the axis and never a shell; the columns are in
// dialect/*/commandroute_test.go.
func runCommandRoute(t *testing.T, src string, reaches Answer) (string, int) {
	t.Helper()
	return runGrammar(t, src, nil, func(r *Runner) {
		r.Semantics.CommandReachesABuiltin = reaches
		// The target's expansion belongs to the process that runs the
		// command, which is the reading this question is only visible
		// under: the column that keeps every write keeps it whichever
		// route the word took, so it could not tell the two apart.
		r.Semantics.RedirectTargetExpandsInTheCommandsProcess = Yes
		r.Semantics.HeredocExpandsInTheCommandsProcess = Yes
	})
}

// A write in the target of a redirection on `command <builtin>` comes back
// where the word reaches the builtin and is lost where it names a program.
func TestWhoseProcessACommandPrefixedRedirectionIsFor(t *testing.T) {
	const src = "unset u\ncommand : > \"${u:=made}\"\nprintf 'u=%s' \"${u-unset}\"\n"
	if out, _ := runCommandRoute(t, src, Yes); !strings.HasSuffix(out, "u=made") {
		t.Errorf("reaching the builtin: out = %q, want the write kept", out)
	}
	if out, _ := runCommandRoute(t, src, No); !strings.HasSuffix(out, "u=unset") {
		t.Errorf("naming a program: out = %q, want the write gone", out)
	}
}

// And the here-document body beside it, which is the same question at the
// other half of the redirection.
func TestWhoseProcessACommandPrefixedHeredocBodyIsFor(t *testing.T) {
	const src = "unset u\ncommand : <<END\n[${u:=made}]\nEND\nprintf 'u=%s' \"${u-unset}\"\n"
	if out, _ := runCommandRoute(t, src, Yes); !strings.HasSuffix(out, "u=made") {
		t.Errorf("reaching the builtin: out = %q, want the write kept", out)
	}
	if out, _ := runCommandRoute(t, src, No); !strings.HasSuffix(out, "u=unset") {
		t.Errorf("naming a program: out = %q, want the write gone", out)
	}
}

// **The noun is a builtin behind the word**, and these are the two pairs that
// say so — each holds one thing fixed and moves the other.
//
// A *function* behind `command` is a failed lookup under either answer, and
// the route is the same under both: bypassing the function table is what the
// utility is for, so "an external command" is not a category this word
// creates and cannot be the noun.
func TestAFunctionBehindTheWordTakesTheSameRouteEitherWay(t *testing.T) {
	const src = "f() { :; }\nunset u\ncommand f > \"${u:=made}\" 2>/dev/null\nprintf 'u=%s' \"${u-unset}\"\n"
	for _, a := range []Answer{Yes, No} {
		if out, _ := runCommandRoute(t, src, a); !strings.HasSuffix(out, "u=unset") {
			t.Errorf("CommandReachesABuiltin=%v: out = %q, want the write gone under both answers", a, out)
		}
	}
}

// And `command -v` and `command -V` run nothing at all, so they stay in this
// shell under either answer — which says the noun is not the *word*
// `command`. The pair is what makes the two rows above evidence rather than
// two readings of one fact.
func TestTheReportingLettersStayInThisShellEitherWay(t *testing.T) {
	for _, letter := range []string{"-v", "-V"} {
		src := "unset u\ncommand " + letter + " : > \"${u:=made}\"\nprintf 'u=%s' \"${u-unset}\"\n"
		for _, a := range []Answer{Yes, No} {
			if out, _ := runCommandRoute(t, src, a); !strings.HasSuffix(out, "u=made") {
				t.Errorf("command %s, CommandReachesABuiltin=%v: out = %q, want the write kept", letter, a, out)
			}
		}
	}
}

// A bare builtin is the control the whole grid rests on: it is this shell's
// under both answers, so anything that moved above moved because of the
// `command` in front of it.
func TestABareBuiltinIsThisShellsEitherWay(t *testing.T) {
	const src = "unset u\n: > \"${u:=made}\"\nprintf 'u=%s' \"${u-unset}\"\n"
	for _, a := range []Answer{Yes, No} {
		if out, _ := runCommandRoute(t, src, a); !strings.HasSuffix(out, "u=made") {
			t.Errorf("CommandReachesABuiltin=%v: out = %q, want the write kept", a, out)
		}
	}
}

// The positive row, so the nulls above are falsifiable: a target that expands
// is opened and the command it belongs to runs, under both answers. The
// command is an external one by path, because that is the only word `command`
// reaches under both — `command :` is `not found` at 127 where the word names
// a program, which is the reference's answer and the reach half of this whole
// question.
func TestACommandPrefixedTargetThatExpandsStillOpens(t *testing.T) {
	for _, a := range []Answer{Yes, No} {
		out, st := runCommandRoute(t, "command /bin/echo RAN < /dev/null\nprintf 'st=%d' \"$?\"\n", a)
		if out != "RAN\nst=0" || st != 0 {
			t.Errorf("CommandReachesABuiltin=%v: out = %q (status %d), want RAN and st=0", a, out, st)
		}
	}
}

// And the reach itself, which is the fact the route follows: where the word
// names a program, a builtin behind it is `not found` at 127 and never runs.
func TestACommandPrefixedBuiltinIsNotFoundWhereTheWordNamesAProgram(t *testing.T) {
	out, _ := runCommandRoute(t, "command : 2>/dev/null\nprintf 'st=%d' \"$?\"\n", No)
	if out != "st=127" {
		t.Errorf("out = %q, want st=127", out)
	}
	if out, _ := runCommandRoute(t, "command :\nprintf 'st=%d' \"$?\"\n", Yes); out != "st=0" {
		t.Errorf("out = %q, want the builtin reached at 0", out)
	}
}

// A `command` carrying no redirection at all asks nothing and says nothing,
// which is why the axis is read here rather than asked: reporting an
// unanswered axis at the route would report it for every such command, and
// again where the command runs.
func TestACommandPrefixWithNoRedirectionIsQuiet(t *testing.T) {
	out, st := runGrammar(t, "command : ; printf 'st=%d' \"$?\"\n", nil, func(r *Runner) {
		r.Semantics.CommandReachesABuiltin = Yes
	})
	if out != "st=0" || st != 0 {
		t.Errorf("out = %q (status %d), want a silent st=0", out, st)
	}
}
