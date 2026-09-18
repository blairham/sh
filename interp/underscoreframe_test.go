// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `$_` across a function call, and the narrowed reading one column has —
// Semantics.UnderscoreMovesBeforeAFunctionBody and
// UnderscoreMovesOnlyBetweenInputCommands, and #3134.
//
// Named for the fields rather than for the shells: which value each preset
// picks is asserted in dialect/underscoreframe_test.go against the panel's own
// bytes. What is asserted here is that each reading can be reached and that
// they are three readings and not one.

// tracks is the vector a shell that has `$_` at all starts from.
//
// Both halves, and they are two axes rather than one: the parameter exists,
// which is what the preset denies because POSIX names none and dash and
// BusyBox ash have not got it, and the last argument moves it. A vector that
// said only the second would be a shell tracking a name it does not have.
func tracks() Semantics {
	s := permissive()
	s.UnderscoreIsAParameterAtAll = Yes
	s.UnderscoreTracksTheLastArgument = Yes
	return s
}

func underscoreOut(t *testing.T, src string, sem Semantics) string {
	t.Helper()
	out, _ := run(t, src, func(r *Runner) { r.Semantics = &sem })
	return out
}

// The probe: a body that reads `$_` at its first line, calls a function of its
// own, and reads it again; then the caller reads it after the call. One script
// for all three readings, because the three differ only in which of those
// three lines they answer differently.
const underscoreProbe = "inner() { :; }\n" +
	"peek() { printf 'body [%s] ' \"$_\"; inner zz; printf 'after [%s] ' \"$_\"; }\n" +
	": outer\n" +
	"peek one two\n" +
	"printf 'call [%s]\\n' \"$_\"\n"

// TestTheCallerReadsTheCallsOwnLastArgument is the unanimous row and the whole
// of #3134's first defect: the body's last command must not leak out.
func TestTheCallerReadsTheCallsOwnLastArgument(t *testing.T) {
	for _, name := range []string{"before the body", "inside the body", "narrowed"} {
		sem := tracks()
		switch name {
		case "before the body":
			sem.UnderscoreMovesBeforeAFunctionBody = Yes
		case "narrowed":
			sem.UnderscoreMovesOnlyBetweenInputCommands = Yes
		}
		got := underscoreOut(t, underscoreProbe, sem)
		if !strings.Contains(got, "call [two]") {
			t.Errorf("%s: %q, want the call's own last argument", name, got)
		}
	}
}

// TestABodyOpensWithWhatTheCallerHad is bash's and ksh93's reading.
func TestABodyOpensWithWhatTheCallerHad(t *testing.T) {
	got := underscoreOut(t, underscoreProbe, tracks())
	if want := "body [outer] after [zz] call [two]\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestABodyOpensWithTheCallsOwnArgument is zsh's, and it is the only line of
// the three that moves when the axis does.
func TestABodyOpensWithTheCallsOwnArgument(t *testing.T) {
	sem := tracks()
	sem.UnderscoreMovesBeforeAFunctionBody = Yes
	got := underscoreOut(t, underscoreProbe, sem)
	if want := "body [two] after [zz] call [two]\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestTheNarrowedReadingMovesForALoneTopLevelCommand is ksh93's, and the probe
// is the one that separates it from the two above: nothing inside the body
// moves it at all.
func TestTheNarrowedReadingMovesForALoneTopLevelCommand(t *testing.T) {
	sem := tracks()
	sem.UnderscoreMovesOnlyBetweenInputCommands = Yes
	got := underscoreOut(t, underscoreProbe, sem)
	if want := "body [outer] after [outer] call [two]\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestTheNarrowedReadingIgnoresEverythingButALoneCommand is the rest of the
// rule, and each row is a construct a simpler reading would have moved for.
//
// The `;`-list is the one that matters most: it is the shape the corpus
// measured this column with, and a shell given the general rule answers `two`
// there where ksh93 answers nothing at all.
func TestTheNarrowedReadingIgnoresEverythingButALoneCommand(t *testing.T) {
	sem := tracks()
	sem.UnderscoreMovesOnlyBetweenInputCommands = Yes
	for _, tc := range []struct {
		name, src, want string
	}{
		{"a lone command", ": alpha\n: beta\n", "beta"},
		{"a semicolon list", ": alpha\n: beta; :\n", "alpha"},
		{"an and-or", ": alpha\n: beta && :\n", "alpha"},
		{"a pipeline", ": alpha\n: beta | :\n", "alpha"},
		{"a bare assignment", ": alpha\nzz=5\n", "alpha"},
		{"a loop", ": alpha\nfor i in one\ndo\n: beta\ndone\n", "alpha"},
		{"a branch", ": alpha\nif : beta\nthen\n:\nfi\n", "alpha"},
		{"a group", ": alpha\n{ : beta\n}\n", "alpha"},
		{"a subshell", ": alpha\n( : beta )\n", "alpha"},
		{"an eval's text", ": alpha\neval ': beta'\n", ": beta"},
		// The gate is cleared by the command that uses it, so what must not
		// happen is a *substitution* in that command's own words consuming
		// it first: the words are expanded before the command records
		// anything, and the shell the substitution runs on is a clone. Both
		// rows would read `alpha` if the outer command had lost its turn.
		{"a substitution in the words", ": alpha\necho \"$(: sub)\" > /dev/null\n", ""},
		{"a substitution in a prefix", ": alpha\nv=$(: sub) : tail\n", "tail"},
	} {
		got := underscoreOut(t, tc.src+"printf '[%s]' \"$_\"\n", sem)
		if got != "["+tc.want+"]" {
			t.Errorf("%s: %q, want [%s]", tc.name, got, tc.want)
		}
	}
}

// TestTheNarrowedReadingIsBlindInsideAnEval is the row read from *inside* the
// nested text rather than after it, and it is the one that says where the
// level is.
//
// Two separate things have to hold for it. The statements an `eval` runs are
// not at the input level, so `: beta` in there moves nothing; and the line's
// own argument — which is the whole eval string — must not arrive early, even
// though the nested text runs through the very same statement loop that
// delivers it. Reading `$_` after the eval cannot see either: the answer is
// the eval string whichever way round it happened.
func TestTheNarrowedReadingIsBlindInsideAnEval(t *testing.T) {
	sem := tracks()
	sem.UnderscoreMovesOnlyBetweenInputCommands = Yes
	got := underscoreOut(t, ": alpha\neval ': beta\nprintf \"in [%s] \" \"$_\"'\n"+
		"printf 'out [%s]' \"$_\"\n", sem)
	if want := "in [alpha] out [: beta\nprintf \"in [%s] \" \"$_\"]"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestTheGeneralReadingMovesForAllOfThem is the control beside it: with the
// narrowing off, every row above moves the parameter. Without this the table
// would pass for a shell that had simply stopped tracking `$_`.
func TestTheGeneralReadingMovesForAllOfThem(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a semicolon list", ": alpha\n: beta; : gamma\n", "gamma"},
		{"an and-or", ": alpha\n: beta && : gamma\n", "gamma"},
		{"a loop", ": alpha\nfor i in one\ndo\n: beta\ndone\n", "beta"},
		{"a group", ": alpha\n{ : beta\n}\n", "beta"},
	} {
		got := underscoreOut(t, tc.src+"printf '[%s]' \"$_\"\n", tracks())
		if got != "["+tc.want+"]" {
			t.Errorf("%s: %q, want [%s]", tc.name, got, tc.want)
		}
	}
}

// A **write** to `$_`, which is a value a script may read back where the name
// is stamped between the commands the shell reads rather than before every
// command. Every write to it was lost: the parameter is produced, so the
// assignment is recorded as a message to the producer and the producer never
// looked. See Runner.underscoreWrittenValue.
func TestAWriteToUnderscoreStandsUntilTheNextInputCommand(t *testing.T) {
	sem := tracks()
	sem.UnderscoreMovesOnlyBetweenInputCommands = Yes
	const src = ": alpha\n" +
		"_=TOP\n" +
		"printf 'write [%s] ' \"$_\"\n" +
		": beta\n" +
		"printf 'stamped [%s] ' \"$_\"\n" +
		"_=SECOND; printf 'same line [%s]\\n' \"$_\"\n"
	want := "write [TOP] stamped [beta] same line [SECOND]\n"
	if got := underscoreOut(t, src, sem); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// And under the other answer the stamp runs before every command, so a write
// is overwritten before anything can read it — which is what makes this the
// same axis rather than a second one.
func TestAWriteToUnderscoreIsLostWhereEveryCommandStampsIt(t *testing.T) {
	sem := tracks()
	sem.UnderscoreMovesOnlyBetweenInputCommands = No
	const src = ": alpha\n_=TOP\nprintf 'write [%s]\\n' \"$_\"\n"
	if got := underscoreOut(t, src, sem); got != "write []\n" {
		t.Errorf("got %q, want %q", got, "write []\n")
	}
}

// The two shapes a script actually writes the name in — throwing a field away
// and looping a fixed number of times — both leave a value the script may then
// read.
func TestTheWaysAScriptWritesUnderscore(t *testing.T) {
	sem := tracks()
	sem.UnderscoreMovesOnlyBetweenInputCommands = Yes
	for _, row := range []struct{ src, want string }{
		{
			"echo \"a b\" | { read _ rest; printf 'read [%s] rest [%s]\\n' \"$_\" \"$rest\"; }",
			"read [a] rest [b]\n",
		},
		{"for _ in loopval; do :; done\nprintf 'loop [%s]\\n' \"$_\"\n", "loop [loopval]\n"},
	} {
		if got := underscoreOut(t, row.src, sem); got != row.want {
			t.Errorf("%s\n got %q\nwant %q", row.src, got, row.want)
		}
	}
}
