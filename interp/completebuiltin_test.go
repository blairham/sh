// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// complete keeps specs it never acts on: a completion file run without a
// terminal registers, lists, and removes them, and observes nothing else.

func TestCompleteRegistersAndReadsBack(t *testing.T) {
	out, st := run(t, `complete -W "a b" foo; complete -F _f bar; complete -p foo; complete`, nil)
	if st != 0 {
		t.Fatalf("status %d (out %q)", st, out)
	}
	if !strings.Contains(out, `complete -W 'a b' foo`) {
		t.Errorf("out = %q, want the spec printed back the way it was written", out)
	}
	if !strings.Contains(out, "complete -F _f bar") {
		t.Errorf("out = %q, want the bare listing to carry every spec", out)
	}
}

func TestCompleteRemovesAndReportsAMiss(t *testing.T) {
	out, st := run(t, `complete -W x foo; complete -r foo; echo "r=$?"; complete -p foo; echo "p=$?"`, nil)
	if !strings.Contains(out, "r=0") || !strings.Contains(out, "p=1") {
		t.Errorf("out = %q (st %d), want removal to succeed and the lookup to miss", out, st)
	}
	if !strings.Contains(out, "no completion specification") {
		t.Errorf("out = %q, want the miss named", out)
	}
}

func TestCompleteIsRemovable(t *testing.T) {
	out, st := run(t, `complete -W x foo`, func(r *Runner) { r.Unregister("complete") })
	if st == 0 {
		t.Errorf("status 0 (out %q), want the dialect without the builtin to refuse it", out)
	}
}

func TestCompleteSurvivesASubshellCopy(t *testing.T) {
	out, _ := run(t, `complete -W x foo; (complete -W y bar); complete`, nil)
	if !strings.Contains(out, "foo") || strings.Contains(out, "bar") {
		t.Errorf("out = %q, want the parent's spec kept and the subshell's its own", out)
	}
}

// `-o` names one of nine completion options, and its argument is a word this
// builtin reads rather than stores.
//
// It went unread until #2412, which is the failure worth a test of its own:
// `complete -o nospace -F _foo foo` registered a completion for a command
// called `nospace` as well as for `foo`, and printed `foo`'s spec back with
// the option's name missing. Both halves are silent — a plausible listing,
// and a spec for a command nobody named.
func TestCompleteReadsTheOptionItIsGiven(t *testing.T) {
	out, st := run(t, `complete -o nospace -F _f foo; complete -p`, nil)
	if st != 0 {
		t.Fatalf("status %d (out %q)", st, out)
	}
	if got, want := out, "complete -o nospace -F _f foo\n"; got != want {
		t.Errorf("out = %q, want %q — and nothing registered for `nospace`", got, want)
	}
}

// The options are printed back first and in their own order, whatever order
// they were written in. Measured: bash renders them sorted and deduplicated
// ahead of the rest of the spec, so `complete -F f -o nospace x` and
// `complete -o nospace -F f x` list identically.
func TestCompleteRendersTheOptionsFirstAndSorted(t *testing.T) {
	for _, src := range []string{
		`complete -F f -o nospace -o dirnames x`,
		`complete -o nospace -o dirnames -F f x`,
		`complete -o dirnames -o nospace -o dirnames -F f x`,
	} {
		out, _ := run(t, src+`; complete -p x`, nil)
		if got, want := out, "complete -o dirnames -o nospace -F f x\n"; got != want {
			t.Errorf("%s: out = %q, want %q", src, got, want)
		}
	}
	// And `+o` takes one off at registration, which is the pair that says
	// the sign is read rather than skipped.
	out, _ := run(t, `complete +o nospace -F f x; complete -p x`, nil)
	if got, want := out, "complete -F f x\n"; got != want {
		t.Errorf("out = %q, want %q", got, want)
	}
}

// An option name neither builtin has is refused at 2 before the table is
// touched, so nothing is registered and nothing is changed.
func TestAnUnknownCompletionOptionNameIsRefused(t *testing.T) {
	out, st := run(t, `complete -o nosuchopt x; echo "st=$?"; complete -p x; echo "p=$?"`, nil)
	if !strings.Contains(out, "complete: nosuchopt: invalid option name") {
		t.Errorf("out = %q, want the name refused", out)
	}
	if !strings.Contains(out, "st=2") || !strings.Contains(out, "p=1") {
		t.Errorf("out = %q (final %d), want 2 and nothing registered", out, st)
	}
	out, _ = run(t, `complete -F f x; compopt -o nosuchopt x; echo "st=$?"; complete -p x`, nil)
	if !strings.Contains(out, "compopt: nosuchopt: invalid option name") ||
		!strings.Contains(out, "st=2") {
		t.Errorf("out = %q, want compopt to refuse the name too", out)
	}
	if !strings.Contains(out, "complete -F f x") {
		t.Errorf("out = %q, want the spec unchanged", out)
	}
}

// `-D`, `-E` and `-I` name the default, empty-line and initial-word specs,
// which live in the same table under names no command can have. bash says
// those names out loud when nothing is registered, and prints the letter back
// where a command's name would go.
func TestTheThreeCompletionSpecsThatAreNotCommands(t *testing.T) {
	for _, tc := range []struct{ letter, internal string }{
		{"-D", "_DefaultCmD_"}, {"-E", "_EmptycmD_"}, {"-I", "_InitialWorD_"},
	} {
		out, _ := run(t, `compopt `+tc.letter+`; echo "st=$?"`, nil)
		if !strings.Contains(out, "compopt: "+tc.internal+": no completion specification") {
			t.Errorf("%s: out = %q, want the internal name said out loud", tc.letter, out)
		}
		if !strings.Contains(out, "st=1") {
			t.Errorf("%s: out = %q, want 1", tc.letter, out)
		}
		out, _ = run(t, `complete `+tc.letter+` -F f; complete -p`, nil)
		if got, want := out, "complete -F f "+tc.letter+"\n"; got != want {
			t.Errorf("%s: out = %q, want %q", tc.letter, got, want)
		}
	}
}

// `compopt` changes the options of a spec that is already registered, and
// `complete -p` is where the change shows.
func TestCompoptMovesTheOptionsOfARegisteredSpec(t *testing.T) {
	out, st := run(t, `complete -F f x; compopt -o nospace -o dirnames x; complete -p x`, nil)
	if st != 0 {
		t.Fatalf("status %d (out %q)", st, out)
	}
	if got, want := out, "complete -o dirnames -o nospace -F f x\n"; got != want {
		t.Errorf("out = %q, want %q", got, want)
	}
	// And `+o` takes one off again, back to the spec as registered.
	out, _ = run(t, `complete -o nospace -F f x; compopt +o nospace x; complete -p x`, nil)
	if got, want := out, "complete -F f x\n"; got != want {
		t.Errorf("out = %q, want %q", got, want)
	}
	// Several names in one call, and a name with no spec is named and does
	// not stop the others.
	out, _ = run(t, `complete -F f a; compopt -o nospace a nosuch; echo "st=$?"; complete -p a`, nil)
	if !strings.Contains(out, "compopt: nosuch: no completion specification") ||
		!strings.Contains(out, "st=1") {
		t.Errorf("out = %q, want the missing name reported at 1", out)
	}
	if !strings.Contains(out, "complete -o nospace -F f a") {
		t.Errorf("out = %q, want the name that had a spec changed anyway", out)
	}
}

// With no `-o` or `+o`, `compopt` lists every option's state for the name —
// all nine, in bash's own order, each with the sign that says whether the
// spec holds it.
func TestCompoptListsEveryOptionsState(t *testing.T) {
	const all = "compopt +o bashdefault +o default +o dirnames +o filenames " +
		"+o fullquote +o noquote +o nosort +o nospace +o plusdirs x\n"
	out, st := run(t, `complete -F f x; compopt x`, nil)
	if st != 0 || out != all {
		t.Errorf("out = %q status %d, want %q", out, st, all)
	}
	out, _ = run(t, `complete -F f x; compopt -o nosort x; compopt x`, nil)
	if !strings.Contains(out, "-o nosort +o nospace") {
		t.Errorf("out = %q, want the one that is on written with a minus", out)
	}
}

// And with no names at all it is the call bash means for inside a completion
// function, which this shell never runs.
func TestCompoptWithNoNameIsNotInACompletionFunction(t *testing.T) {
	out, st := run(t, `complete -F f x; compopt -o nospace; echo "st=$?"`, nil)
	if !strings.Contains(out, "compopt: not currently executing completion function") {
		t.Errorf("out = %q, want the refusal", out)
	}
	if !strings.Contains(out, "st=1") {
		t.Errorf("out = %q (final %d), want 1", out, st)
	}
}

// A subshell's change to a spec stays in the subshell, which is what the deep
// copy in clonetables.go is for: the options are a slice, and a shallow copy
// would let the child write into the parent's backing array.
func TestCompoptInASubshellStaysThere(t *testing.T) {
	out, _ := run(t, `complete -F f x; ( compopt -o nospace x ); complete -p x`, nil)
	if got, want := out, "complete -F f x\n"; got != want {
		t.Errorf("out = %q, want %q", got, want)
	}
}
