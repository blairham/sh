// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Turning off something this shell was never doing is a request that has been
// granted, and stopping a script over it is not honesty but noise.
//
// `set +o posix` is the thirteenth line of Homebrew's own `brew`, and it used
// to end the run there with `invalid option name`.
func TestTurningOffWhatThisShellNeverDoesSucceeds(t *testing.T) {
	for _, name := range []string{"posix", "notify", "vi"} {
		t.Run(name, func(t *testing.T) {
			// The status of `set` itself, and no complaint of any kind.
			// Asserting only that the script carried on passed a mutant
			// that refused every one of these: the refusal is not fatal
			// under these semantics, so the `echo` after it still ran.
			out, _ := run(t, "set +o "+name+"\necho \"st=$?\"\n", withExtras)
			if !strings.Contains(out, "st=0") {
				t.Errorf("set +o %s gave %q, want status 0", name, out)
			}
			if strings.Contains(out, "not implemented") || strings.Contains(out, "invalid option name") {
				t.Errorf("set +o %s complained: %q", name, out)
			}
		})
	}
}

// listedState reads one row out of a `set -o` listing, for the assertions
// about a state rather than a status. It answers "on", "off", or the empty
// string where the listing has no such row; the *last* listing a script
// printed is the one read, so a subshell's rows do not hide the parent's.
//
// The split is on the name rather than on whitespace, because the substrate's
// own listing pads to a fixed width and a name longer than that runs straight
// into its state — `interactive-commentsoff` is a row this shell writes. The
// state has to be one of the two words for the row to count, so a longer name
// beginning with a shorter one is not mistaken for it.
func listedState(out, name string) string {
	state := ""
	for _, line := range strings.Split(out, "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), name)
		if !ok {
			continue
		}
		switch strings.TrimSpace(rest) {
		case "on":
			state = "on"
		case "off":
			state = "off"
		}
	}
	return state
}

// And turning one *on* is remembered rather than refused, which is the answer
// #3128 replaced a refusal with.
//
// A refusal was the honest answer while the listing was short. It stopped
// being one once the listing carried a shell's whole roster, because the
// listing is a capture surface: `eval "$(set +o)"` writes back what it read,
// so a row the shell advertises and then declines is a line it hands out and
// rejects. Measured 2026-09-16 — every shell in the panel that lists `notify`
// takes it in both directions at 0, and so does every other name any of them
// lists, bar the handful about being interactive.
//
// What recording promises is exactly this much: the request succeeds, the
// listing reports it back, and nothing else in the shell reads it. So the
// state is what is asserted rather than the status alone — a status of 0 on
// its own cannot tell a request that was remembered from one dropped on the
// floor, and one name in ksh93's roster really is the second of those (see
// TestAnInertOptionIsTakenAndDoesNotMove).
//
// `posix` and `vi` left this kind by being *built*, which is the other way
// out: TestPosixModeMovesAnAxis and
// TestTheTwoEditingModesAreOneStateWithThreeValues hold those two to it.
func TestANameThisShellDoesNotDoIsRememberedRatherThanRefused(t *testing.T) {
	for _, name := range []string{"notify", "ignoreeof", "nolog"} {
		t.Run(name, func(t *testing.T) {
			// The status of `set` itself, which a later command would
			// otherwise replace — the first version of this test asserted
			// the script's status and passed on the strength of the `echo`
			// after it.
			out, _ := run(t, "set -o "+name+"\necho \"st=$?\"\nset -o\n", withExtras)
			if strings.Contains(out, "not implemented") || !strings.Contains(out, "st=0") {
				t.Errorf("set -o %s said %q, want it taken at 0", name, out)
			}
			if got := listedState(out, name); got != "on" {
				t.Errorf("set -o %s left the row %q, want on (listing %q)", name, got, out)
			}
			// And off again, which is the half a one-way write would pass.
			out, _ = run(t, "set -o "+name+"\nset +o "+name+"\nset -o\n", withExtras)
			if got := listedState(out, name); got != "off" {
				t.Errorf("set +o %s left the row %q, want off (listing %q)", name, got, out)
			}
		})
	}
}

// The refusal is still there for a name with nothing behind it at all.
//
// It is the substrate's answer to a name a dialect declared and the table
// knows nothing about — every entry in the table is now implemented, recorded
// or read-only, so the two names that reach this are `rc` and `login_shell`,
// which report a fact about the invocation and have no state to write. The one
// shell that lists them refuses them out loud in both directions and declares
// so with AddImmovableSetOptions, which is why the branch is reached here by a
// runner that declares the name and not the refusal.
//
// Pinned rather than left to the table, because a branch nothing can reach is
// dead data that reads as a rule: this is where an embedder's own name lands
// the day it is declared without an answer.
func TestADeclaredNameWithNothingBehindItIsStillRefused(t *testing.T) {
	setup := func(r *Runner) { r.AddSetOptions("rc") }
	out, _ := run(t, "set -o rc\necho \"st=$?\"\n", setup)
	if !strings.Contains(out, "not implemented") || !strings.Contains(out, "st=2") {
		t.Errorf("set -o rc said %q, want it refused as unimplemented at 2", out)
	}
	// And the other direction is the state it is already in, so it is granted
	// — which is what says the refusal is about the promise and not the name.
	out, _ = run(t, "set +o rc\necho \"st=$?\"\n", setup)
	if strings.Contains(out, "not implemented") || !strings.Contains(out, "st=0") {
		t.Errorf("set +o rc said %q, want it granted at 0", out)
	}
}

// A recorded state is the subshell's, exactly as an implemented one is.
//
// The states behind the implemented options are plain fields on the runner and
// a clone copies them by value; a recorded one lives in a table, and a table a
// clone shares is one state with two shells writing it. See
// interp/clonetables.go, which is where that is enforced rather than
// remembered.
func TestARecordedOptionDoesNotEscapeASubshell(t *testing.T) {
	// The parent moves a recorded name *first*, and that is the whole of what
	// makes this discriminating. The store is allocated lazily, and
	// maps.Clone keeps a nil map nil — so a parent that has never recorded
	// anything hands the subshell a nil, the subshell builds a table of its
	// own, and a shared field leaks nothing yet. Written the other way round
	// this test passed with the clone deleted. See interp/clonetables.go,
	// which is where that trap is recorded.
	const seed = "set -o ignoreeof\n"
	inside, _ := run(t, seed+"(set -o notify; set -o)\n", withExtras)
	if got := listedState(inside, "notify"); got != "on" {
		t.Errorf("inside the subshell notify was %q, want on", got)
	}
	after, _ := run(t, seed+"(set -o notify)\nset -o\n", withExtras)
	if got := listedState(after, "notify"); got != "off" {
		t.Errorf("after the subshell notify was %q, want it left where the parent had it", got)
	}
}

// And a name a dialect declares inert is taken and does not move.
//
// ksh93's `privileged` is the one: `set -o privileged` is 0 with nothing on
// standard error and the row still reads off afterwards, and `ksh -p` reports
// it off too — so the request is granted and the state is out of a script's
// reach. That is neither a refusal nor a move, and it is the reason this test
// reads the listing rather than the status. See Runner.AddInertSetOptions.
func TestAnInertOptionIsTakenAndDoesNotMove(t *testing.T) {
	setup := func(r *Runner) {
		withExtras(r)
		r.AddInertSetOptions("privileged")
	}
	out, _ := run(t, "set -o privileged\necho \"st=$?\"\nset -o\n", setup)
	if strings.Contains(out, "not implemented") || !strings.Contains(out, "st=0") {
		t.Errorf("set -o privileged said %q, want it taken at 0", out)
	}
	if got := listedState(out, "privileged"); got != "off" {
		t.Errorf("set -o privileged left the row %q, want it still off (listing %q)", got, out)
	}
	// Without the declaration the same name is recorded and does move, which
	// is what says the declaration is doing the work rather than the table.
	out, _ = run(t, "set -o privileged\nset -o\n", withExtras)
	if got := listedState(out, "privileged"); got != "on" {
		t.Errorf("an undeclared privileged left the row %q, want on (listing %q)", got, out)
	}
}

// A name this shell does not have at all is a different complaint, and keeps
// the wording it had.
func TestANameThisShellDoesNotHaveIsStillInvalid(t *testing.T) {
	for _, name := range []string{"bogus", "notanoption"} {
		t.Run(name, func(t *testing.T) {
			out, _ := run(t, "set +o "+name+"\necho \"st=$?\"\n", withExtras)
			if !strings.Contains(out, "invalid option name") {
				t.Errorf("said %q, want an invalid name", out)
			}
			if strings.Contains(out, "not implemented") {
				t.Errorf("said %q, want it called invalid rather than unimplemented", out)
			}
			if !strings.Contains(out, "st=2") {
				t.Errorf("said %q, want status 2", out)
			}
		})
	}
}

// A name whose recorded state starts *on* is a request in the plus direction,
// and it is taken there too.
//
// Comments are honored wherever they are written here, so the row is on and
// nothing reads it — which used to make `set +o interactive-comments` a
// refusal. bash takes that word at 0, measured 2026-09-16, and it is the one
// name in bash's own listing this shell refused in the plus direction: the
// half of #3128 that only a sweep of a listing in *both* directions finds.
//
// It used to be `braceexpand` standing here, and that was the wrong name for
// the shape: braces are something this shell *does*, so the switch beside them
// is buildable and was built in #1856. The names left in this kind are the
// ones with nothing behind them to move.
func TestANameThisShellAlreadyDoesIsTakenInBothDirections(t *testing.T) {
	setup := func(r *Runner) { r.AddSetOptions("interactive-comments") }
	out, st := run(t, "set -o interactive-comments\necho \"st=$?\"\n", setup)
	if st != 0 || !strings.Contains(out, "st=0") {
		t.Errorf("set -o interactive-comments gave %q (status %d), want it accepted", out, st)
	}
	out, _ = run(t, "set +o interactive-comments\necho \"st=$?\"\nset -o\n", setup)
	if strings.Contains(out, "not implemented") || !strings.Contains(out, "st=0") {
		t.Errorf("set +o interactive-comments gave %q, want it taken at 0", out)
	}
	if got := listedState(out, "interactive-comments"); got != "off" {
		t.Errorf("set +o interactive-comments left the row %q, want off (listing %q)", got, out)
	}
}

// And the name that left that kind: brace expansion moves in both directions,
// so the request is acted on rather than remembered.
//
// Both directions on one run, because a shell that stopped expanding and could
// not start again would pass the half that only turns it off — and `noexec` is
// the option that really is one-way, which is what says this one had to be
// asserted rather than assumed.
func TestBraceExpansionIsASwitchAndNotOneWay(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		{name: "off", src: "set +o braceexpand\necho {a,b}\n", want: "{a,b}\n"},
		{
			name: "off and on again",
			src:  "set +o braceexpand\nset -o braceexpand\necho {a,b}\n",
			want: "a b\n",
		},
		{
			// The redirection target counts braces of its own — two names
			// are as ambiguous as none — so the switch has to be read there
			// too and not only in a command's arguments.
			name: "a redirection target is one name while it is off",
			src:  "set +o braceexpand\n: > {a,b}\nprintf '[%s]' *\n",
			want: "[{a,b}]",
		},
		{
			// And back to two names once it is on again, which is what says
			// the target reads the switch rather than having been given up on.
			name:   "and two names again once it is back on",
			src:    "set +o braceexpand\nset -o braceexpand\n: > {a,b}\n",
			want:   "sh: {a,b}: ambiguous redirect\n",
			status: 2,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			out, st := run(t, tc.src, func(r *Runner) {
				// The axes a redirection target touches, so that a refusal
				// here is about the switch and not about splitting or
				// matching in general — see redirecttarget_test.go.
				sem := testSemantics()
				sem.SplitParamExpansion = Yes
				sem.SplitCommandSubstitution = Yes
				sem.GlobExpansionResults = Yes
				sem.GlobNoMatchIsError = No
				sem.RedirectTargetIsAnOrdinaryWord = Yes
				r.Semantics, r.Dir = &sem, dir
				withExtras(r)
			})
			if out != tc.want || st != tc.status {
				t.Errorf("got %q (status %d), want %q (status %d)", out, st, tc.want, tc.status)
			}
		})
	}
}

// The name a dialect does not declare is not available even though another
// dialect has it, which is what makes the list a dialect's answer.
func TestAnExtraNameNeedsDeclaring(t *testing.T) {
	// No AddSetOptions call at all, so only the common names exist.
	out, _ := run(t, "set +o posix\n", nil)
	if !strings.Contains(out, "invalid option name") {
		t.Errorf("said %q, want posix unknown to a shell that never declared it", out)
	}
	// And a common name is there without any declaring.
	if out, st := run(t, "set +o noexec\necho after\n", nil); st != 0 || !strings.Contains(out, "after") {
		t.Errorf("set +o noexec gave %q (status %d), want a common name to need no declaring", out, st)
	}
}

// `set -a` marks what is assigned afterwards for the environment, which is
// the one unanimous name in this change that this shell now really has.
func TestAllexportMarksAssignmentsForTheEnvironment(t *testing.T) {
	out, st := run(t, "set -a\nFOO=bar\n/bin/sh -c 'echo [$FOO]'\n", withExtras)
	if st != 0 || !strings.Contains(out, "[bar]") {
		t.Errorf("got %q (status %d), want the assignment carried to the child", out, st)
	}
	// And without it the child sees nothing, which is what says the option
	// did the work rather than something else.
	if out, _ := run(t, "FOO=bar\n/bin/sh -c 'echo [$FOO]'\n", withExtras); !strings.Contains(out, "[]") {
		t.Errorf("got %q, want an unexported assignment to stay behind", out)
	}
	// Turned off again, it stops applying.
	if out, _ := run(t, "set -a\nset +a\nFOO=bar\n/bin/sh -c 'echo [$FOO]'\n", withExtras); !strings.Contains(out, "[]") {
		t.Errorf("got %q, want `set +a` to stop marking assignments", out)
	}
}

// withExtras gives the runner the names one shell in the panel has beyond the
// common ones, which is what a dialect does.
func withExtras(r *Runner) {
	// `emacs` and `nolog` are declared here since #3366 took them out of the
	// substrate's unanimous table: BusyBox ash has neither, so a shell that
	// wants them says so, the way a dialect does.
	r.AddSetOptions("braceexpand", "emacs", "hashall", "nolog", "posix", "privileged")
}

// The wording, the status and the usage line that follows are the dialect's,
// and only one of the panel answers each of them differently. Asserted here
// with the answers spelled out, because a `go test` run does not reach the
// corpus and these were invisible to it.
func TestARefusedNameIsWordedByTheDialect(t *testing.T) {
	for _, c := range []struct {
		name string
		dg   Diagnostics
		want []string
		gone string
	}{
		{
			"a shell with its own words and status",
			Diagnostics{SetInvalidOptionName: "set: no such option: %[1]s", SetInvalidOptionNameStatus: 1},
			[]string{"no such option: bogus", "st=1"},
			"invalid option name",
		},
		{
			"a shell that follows it with a usage line",
			Diagnostics{
				BuiltinUsage:              map[string]string{"set": "Usage: set [-abc]"},
				SetInvalidOptionNameUsage: true,
			},
			[]string{"invalid option name", "Usage: set [-abc]", "st=2"},
			"",
		},
		{
			"and the default, which needs neither",
			Diagnostics{},
			[]string{"invalid option name", "st=2"},
			"Usage:",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out := runWorded(t, "set +o bogus\necho \"st=$?\"\n", c.dg)
			for _, w := range c.want {
				if !strings.Contains(out, w) {
					t.Errorf("said %q, want %q in it", out, w)
				}
			}
			if c.gone != "" && strings.Contains(out, c.gone) {
				t.Errorf("said %q, want %q not in it", out, c.gone)
			}
		})
	}
}

// runWith runs a script with the wordings a dialect would supply.
func runWorded(t *testing.T, src string, dg Diagnostics) string {
	t.Helper()
	var buf strings.Builder
	sem := PosixSemantics()
	// Neither spelling fatal here, so that what `set` reported can still be
	// printed. Both, because these rows drive the letter and the name alike.
	sem.BadSetOptionNameFatal = No
	sem.BadSetOptionLetterFatal = No
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "sh"})
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// TestSetNReadsAndNeverRuns — `set -n` is the syntax-check option: commands
// after it are read and never executed, nothing turns it back off, and the
// script still ends at 0. All four shells agree.
func TestSetNReadsAndNeverRuns(t *testing.T) {
	out, st := run(t, `set -n; echo nope; set +n; echo plusn`, nil)
	if out != "" || st != 0 {
		t.Errorf("out=%q st=%d, want silence at 0", out, st)
	}
	// The subshell and the substitution inherit it.
	out, _ = run(t, `set -n; (echo sub); echo $(echo inner)`, nil)
	if out != "" {
		t.Errorf("out=%q, want the option to reach every runner", out)
	}
	// And $- carries the letter while it is on... which nothing can observe
	// from inside, so it is asserted from before: without -n the letter is
	// absent.
	out, _ = run(t, `case $- in *n*) echo has-n;; *) echo no-n;; esac`, nil)
	if !strings.Contains(out, "no-n") {
		t.Errorf("out=%q, want no letter without the option", out)
	}
}

// TestSetTraceLettersAreAnAxis — -E carries the ERR trap into functions and
// -T carries DEBUG and RETURN; only a dialect with the letter takes it.
//
// The two letters are two axes, so the last case here is the one that could
// not be written while they were one field: a dialect holding `E` and refusing
// `T` (#3366).
func TestSetTraceLettersAreAnAxis(t *testing.T) {
	withE := func(r *Runner) {
		sem := CoreSemantics()
		sem.SetHasTheErrtraceLetter = Yes
		sem.TrapHasErrCondition = Yes
		sem.ErrTrapRunsInsideFunctions = No
		sem.TrapBodyRunsWhatParsed = Yes
		r.Semantics = &sem
	}
	out, _ := run(t, `set -E; trap "echo ERR" ERR; f(){ false; }; f; true`, withE)
	if !strings.Contains(out, "ERR") {
		t.Errorf("out=%q, want -E to carry the trap into the function", out)
	}
	out, _ = run(t, `trap "echo ERR" ERR; f(){ false; }; f; true`, withE)
	if strings.Contains(out, "ERR\nERR") {
		t.Errorf("out=%q, want the function firing bounded without -E", out)
	}
	out, _ = run(t, `set -T; echo "st=$?"`, func(r *Runner) {
		sem := CoreSemantics()
		sem.SetHasTheErrtraceLetter = No
		sem.SetHasTheFunctraceLetter = No
		r.Semantics = &sem
	})
	if !strings.Contains(out, "set: -T: invalid option") || !strings.Contains(out, "st=2") {
		t.Errorf("out=%q, want the dialect without the letters to refuse at 2", out)
	}
	// One letter and not the other, which is the shape a single field could
	// not hold: `set -E` is taken and `set -T` refused on the same runner.
	out, _ = run(t, `set -E; echo "e=$?"; set -T; echo "t=$?"`, func(r *Runner) {
		sem := CoreSemantics()
		sem.SetHasTheErrtraceLetter = Yes
		sem.SetHasTheFunctraceLetter = No
		r.Semantics = &sem
	})
	if !strings.Contains(out, "e=0") || !strings.Contains(out, "set: -T: invalid option") {
		t.Errorf("out=%q, want -E taken and -T refused on one runner", out)
	}
}

// TestLongAndShortSpellingsAreTheSameOption — an option reachable by letter
// is reachable by name with the same behavior, because one table drives both.
func TestLongAndShortSpellingsAreTheSameOption(t *testing.T) {
	// `set -o noexec` reads and never runs, exactly as `set -n` does.
	out, st := run(t, "echo before\nset -o noexec\necho after\n", nil)
	if out != "before\n" || st != 0 {
		t.Errorf("out=%q st=%d, want only what ran before the option", out, st)
	}
	// And it is one-way under the long spelling too.
	out, _ = run(t, "set -o noexec; set +o noexec; echo back", nil)
	if out != "" {
		t.Errorf("out=%q, want the option to stay on", out)
	}
	// `set -o verbose` sets the state `set -v` sets, which `$-` reports.
	// The echoing itself lives in the front end, so the letter is the
	// observable half here.
	out, _ = run(t, `set -o verbose; case $- in *v*) echo has-v;; *) echo no-v;; esac`, nil)
	if !strings.Contains(out, "has-v") {
		t.Errorf("out=%q, want the letter reported", out)
	}
	out, _ = run(t, `set -o verbose; set +o verbose; case $- in *v*) echo has-v;; *) echo no-v;; esac`, nil)
	if !strings.Contains(out, "no-v") {
		t.Errorf("out=%q, want the long spelling to turn it back off", out)
	}
}

// TestMonitorWithoutATerminalIsAnAxis — bash and ksh93 grant `set -m` to a
// script, dash declines it in a remark that is not a failure, and zsh refuses
// it outright. The shapes are asserted here by axis and diagnostic, never by
// shell name.
func TestMonitorWithoutATerminalIsAnAxis(t *testing.T) {
	granted := func(r *Runner) {
		sem := CoreSemantics()
		sem.MonitorNeedsATerminal = No
		r.Semantics = &sem
	}
	out, st := run(t, `set -m; echo "st=$?"; case $- in *m*) echo has-m;; *) echo no-m;; esac`, granted)
	if st != 0 || !strings.Contains(out, "st=0") || !strings.Contains(out, "has-m") {
		t.Errorf("out=%q st=%d, want the option granted and reported", out, st)
	}
	// Both spellings, one table.
	out, _ = run(t, `set -o monitor; set +m; case $- in *m*) echo has-m;; *) echo no-m;; esac`, granted)
	if !strings.Contains(out, "no-m") {
		t.Errorf("out=%q, want the letter to turn off what the name turned on", out)
	}

	// Needing a terminal and having none: the default shape is a remark —
	// the option stays off and `set` still succeeds.
	remarks := func(r *Runner) {
		sem := CoreSemantics()
		sem.MonitorNeedsATerminal = Yes
		r.Semantics = &sem
	}
	out, st = run(t, `set -m; echo "st=$?"; case $- in *m*) echo has-m;; *) echo no-m;; esac`, remarks)
	if st != 0 || !strings.Contains(out, "st=0") || !strings.Contains(out, "no-m") {
		t.Errorf("out=%q st=%d, want a remark with the option left off", out, st)
	}
	if !strings.Contains(out, "without a terminal") {
		t.Errorf("out=%q, want the denial said out loud", out)
	}
	// A nonzero denial status makes it a failure, worded by the dialect and
	// echoing the spelling that asked; fatality follows the same axis as any
	// other refused `set`.
	refuses := func(r *Runner) {
		sem := CoreSemantics()
		sem.MonitorNeedsATerminal = Yes
		// Both, because a denied `set -m` is the one refusal that arrives
		// under either spelling and asks the axis for the one that did.
		sem.BadSetOptionNameFatal = No
		sem.BadSetOptionLetterFatal = No
		r.Semantics = &sem
		r.Diagnostics = &Diagnostics{MonitorDenied: "can't change option: %[1]s", MonitorDeniedStatus: 1}
	}
	out, _ = run(t, "set -m\necho \"st=$?\"\nset -o monitor\necho \"st=$?\"\n", refuses)
	if !strings.Contains(out, "can't change option: -m") || !strings.Contains(out, "can't change option: monitor") {
		t.Errorf("out=%q, want each refusal to echo its own spelling", out)
	}
	if !strings.Contains(out, "st=1") {
		t.Errorf("out=%q, want the denial's own status", out)
	}
	fatally := func(r *Runner) {
		refuses(r)
		sem := *r.Semantics
		sem.BadSetOptionNameFatal = Yes
		sem.BadSetOptionLetterFatal = Yes
		sem.FatalErrorStatusIsOne = Yes
		r.Semantics = &sem
	}
	out, st = run(t, "set -m\necho survived\n", fatally)
	if strings.Contains(out, "survived") || st != 1 {
		t.Errorf("out=%q st=%d, want the refusal to end the script at 1", out, st)
	}
	// Turning it off is granted whatever the dialect wants for turning it on.
	out, _ = run(t, `set +m; echo "st=$?"; set +o monitor; echo "st2=$?"`, remarks)
	if !strings.Contains(out, "st=0") || !strings.Contains(out, "st2=0") {
		t.Errorf("out=%q, want off granted everywhere", out)
	}
	// A front end that said this runner has a terminal makes the question
	// moot, and it is never asked. Runner.Terminal and not JobControl: having
	// somebody to announce a job to is a prompt and having a terminal is any
	// route started from one, and measured on a pseudo-terminal every shell
	// in the panel grants `set -m` inside a plain `-c` string (#1720).
	out, st = run(t, `set -m; echo "st=$?"`, func(r *Runner) {
		remarks(r)
		r.Terminal = true
	})
	if st != 0 || !strings.Contains(out, "st=0") || strings.Contains(out, "terminal") {
		t.Errorf("out=%q st=%d, want the option granted at a terminal", out, st)
	}
}

// TestTheHLetterIsAnAxis — three shells have `set -h` and no two mean quite
// the same thing by it; the fourth refuses the letter.
func TestTheHLetterIsAnAxis(t *testing.T) {
	// The names involved, declared the way a dialect declares them.
	declare := func(sem Semantics) func(*Runner) {
		return func(r *Runner) {
			r.Semantics = &sem
			r.AddSetOptions("hashall", "trackall", "histignoredups")
		}
	}
	tracking := CoreSemantics()
	tracking.SetHasTheHLetter = Yes
	tracking.SetHLetterTracksCommands = Yes
	// The letter turns command tracking on under both of its long names,
	// which is one state: turning either name off turns the letter's work
	// off with it.
	out, st := run(t, "set -h\nset +o\n", declare(tracking))
	if st != 0 || !strings.Contains(out, "set -o hashall") || !strings.Contains(out, "set -o trackall") {
		t.Errorf("out=%q st=%d, want the letter to set command tracking", out, st)
	}
	if !strings.Contains(out, "set +o histignoredups") {
		t.Errorf("out=%q, want the history option untouched", out)
	}
	out, _ = run(t, "set -h\nset +o trackall\nset +o\n", declare(tracking))
	if !strings.Contains(out, "set +o hashall") {
		t.Errorf("out=%q, want one state behind both names", out)
	}

	history := tracking
	history.SetHLetterTracksCommands = No
	out, st = run(t, "set -h\nset +o\n", declare(history))
	if st != 0 || !strings.Contains(out, "set -o histignoredups") || !strings.Contains(out, "set +o hashall") {
		t.Errorf("out=%q st=%d, want the letter to be the history option's", out, st)
	}

	refused := CoreSemantics()
	refused.SetHasTheHLetter = No
	out, _ = run(t, "set -h\necho \"st=$?\"\n", declare(refused))
	if !strings.Contains(out, "set: -h: invalid option") || !strings.Contains(out, "st=2") {
		t.Errorf("out=%q, want the dialect without the letter to refuse at 2", out)
	}
}

// TestPipefailIsListedWhereDeclared — the option is real everywhere the axis
// says it exists, and the listings have to say so: `set -o` shows the row and
// `set +o` writes it back re-inputtable, in both states.
func TestPipefailIsListedWhereDeclared(t *testing.T) {
	withPipefail := func(r *Runner) {
		sem := CoreSemantics()
		sem.PipefailOption = Yes
		r.Semantics = &sem
		r.AddSetOptions("pipefail")
	}
	out, st := run(t, "set +o\n", withPipefail)
	if st != 0 || !strings.Contains(out, "set +o pipefail") {
		t.Errorf("out=%q st=%d, want the off state written back", out, st)
	}
	out, _ = run(t, "set -o pipefail\nset +o\n", withPipefail)
	if !strings.Contains(out, "set -o pipefail") {
		t.Errorf("out=%q, want the on state written back", out)
	}
	out, _ = run(t, "set -o\n", withPipefail)
	if !strings.Contains(out, "pipefail") {
		t.Errorf("out=%q, want a pipefail row in the table", out)
	}
	// Where the dialect never declared the name, the listings stay silent
	// about it — the axis decides what `set -o pipefail` says, and the
	// vocabulary decides what the listings hold.
	out, _ = run(t, "set +o\n", func(r *Runner) {
		sem := CoreSemantics()
		sem.PipefailOption = Yes
		r.Semantics = &sem
	})
	if strings.Contains(out, "pipefail") {
		t.Errorf("out=%q, want no pipefail line without a declaration", out)
	}
}

// TestARefusedLetterReportsTheDialectsStatus is #483: the letter and the name
// are one question with one answer, and only the name could carry it. `set -q`
// reported 2 whatever the dialect said, while `set -o nosuchoption` under the
// same dialect reported 1 — one shell answering itself two ways.
//
// Asserted against a *named* status rather than against a shell, since a shell
// is a value here and this package does not know any: what the test says is
// that the value travels.
func TestARefusedLetterReportsTheDialectsStatus(t *testing.T) {
	for _, c := range []struct {
		name           string
		dg             Diagnostics
		letter, nameSt string
	}{
		{"a shell with a status of its own", Diagnostics{SetInvalidOptionNameStatus: 1, SetInvalidOptionLetterStatus: 1}, "st=1", "st=1"},
		{"and the default", Diagnostics{}, "st=2", "st=2"},
		// The row #2629 added, and the one that would pass under either
		// reading if the two above stood alone: a dialect that answers the
		// two spellings differently has to produce both answers from one
		// run. BusyBox ash is that shell.
		{"a shell that splits them", Diagnostics{SetInvalidOptionNameStatus: 1, SetInvalidOptionLetterStatus: 2}, "st=2", "st=1"},
	} {
		t.Run(c.name, func(t *testing.T) {
			// Both spellings in one run, so that a change teaching one of
			// them and not the other fails — and in a fixed order, so that
			// which answer landed where is part of the assertion.
			out := runWorded(t, "set -q\necho \"st=$?\"\nset -o bogus\necho \"st=$?\"\n", c.dg)
			var got []string
			for _, line := range strings.Split(out, "\n") {
				if strings.HasPrefix(line, "st=") {
					got = append(got, line)
				}
			}
			want := []string{c.letter, c.nameSt}
			if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
				t.Errorf("said %q, want the letter to report %q and the name %q", out, c.letter, c.nameSt)
			}
		})
	}
}

// opposite is the other Answer, for a test that wants one axis answered one
// way and the axis beside it the other — so that a reading through the wrong
// field of the pair produces the wrong output rather than the right one by
// coincidence.
func opposite(a Answer) Answer {
	if a == Yes {
		return No
	}
	return Yes
}

// TestARefusedLetterEndsTheScriptWhereTheDialectSaysSo: the other half of
// routing the letter the way the name goes. Three of the panel end the script
// on a refused `set` option and one carries on, and the letter used to carry
// on everywhere — so a script that asked for an option its shell does not have
// ran the rest of itself in three shells that would have stopped.
func TestARefusedLetterEndsTheScriptWhereTheDialectSaysSo(t *testing.T) {
	for _, c := range []struct {
		name  string
		fatal Answer
		want  string
	}{
		{"a shell that stops", Yes, ""},
		{"a shell that carries on", No, "after\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var buf strings.Builder
			sem := PosixSemantics()
			// The letter's axis, because `set -q` is a letter. The name's is
			// answered the other way round on purpose: a run that took the
			// name's answer here would print the opposite of what is wanted,
			// which is what makes this test read the field it names.
			sem.BadSetOptionLetterFatal = c.fatal
			sem.BadSetOptionNameFatal = opposite(c.fatal)
			dg := Diagnostics{}
			r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &strings.Builder{}, Semantics: &sem, Diagnostics: &dg, Name: "sh"})
			f, err := syntax.Parse("set -q\necho after\n", syntax.Core())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatal(err)
			}
			if buf.String() != c.want {
				t.Errorf("got %q, want %q", buf.String(), c.want)
			}
		})
	}
}

// TestSetOptionLettersReportsAStatus is the front end's half of #483, at the
// call it makes. It returned a bool, so the one status the caller had was the
// only one an invocation could exit with however the dialect answered.
func TestSetOptionLettersReportsAStatus(t *testing.T) {
	newRunner := func(status int) *Runner {
		sem := PosixSemantics()
		sem.BadSetOptionLetterFatal = No
		// The letter's status alone, and the name's deliberately left at the
		// default: these are letters, and a front end reading the name's
		// field for them would answer 2 whatever the dialect said — which is
		// the #483 bug in the shape #2629 could have reintroduced.
		dg := Diagnostics{SetInvalidOptionLetterStatus: status}
		return newTestRunner(t, &Runner{Stdout: &strings.Builder{}, Stderr: &strings.Builder{}, Semantics: &sem, Diagnostics: &dg, Name: "sh"})
	}
	if got := newRunner(0).SetOptionLetters("e", true); got != 0 {
		t.Errorf("a letter this shell has gave %d, want 0", got)
	}
	if got := newRunner(1).SetOptionLetters("q", true); got != 1 {
		t.Errorf("a refused letter gave %d, want the dialect's 1", got)
	}
	if got := newRunner(0).SetOptionLetters("q", true); got != 2 {
		t.Errorf("a refused letter under a dialect with no answer gave %d, want 2", got)
	}
	// The letters before the refused one still applied, which is why a bundle
	// is one call: `sh -eq` is errexit and then a refusal, not neither.
	r := newRunner(1)
	if got := r.SetOptionLetters("eq", true); got != 1 {
		t.Errorf("a bundle ending in a refused letter gave %d, want 1", got)
	}
}

// The words a refused option *letter* is given are the dialect's, and the
// panel spells them four ways. Named by what each answer does rather than by
// the shell that gives it: this package knows no shells.
//
// The sign is the part that needs two verbs. Two of the panel echo back the
// `+` of `set +q` and two write `-q` whichever way they were asked, so a
// wording takes the spelling as written and the bare letter and uses the one
// it means (#598).
func TestARefusedLetterIsWordedByTheDialect(t *testing.T) {
	for _, c := range []struct {
		name string
		dg   Diagnostics
		want []string
		gone string
	}{
		{
			"a shell that echoes the sign it was asked with",
			Diagnostics{SetInvalidOptionLetter: "set: %[1]s: unknown option"},
			[]string{"set: -q: unknown option", "set: +j: unknown option"},
			"invalid option",
		},
		{
			"a shell that writes a dash whichever it was asked with",
			Diagnostics{SetInvalidOptionLetter: "set: Illegal option -%[2]s"},
			[]string{"set: Illegal option -q", "set: Illegal option -j"},
			"+j",
		},
		{
			"a shell that follows the letter with the builtin's usage line",
			Diagnostics{BuiltinUsage: map[string]string{"set": "Usage: set [-abc]"}},
			[]string{"set: -q: invalid option", "Usage: set [-abc]"},
			"",
		},
		{
			"and the default, which needs neither",
			Diagnostics{},
			[]string{"set: -q: invalid option", "set: +j: invalid option"},
			"Usage:",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out := runWorded(t, "set -q\nset +j\n", c.dg)
			for _, w := range c.want {
				if !strings.Contains(out, w) {
					t.Errorf("said %q, want it to contain %q", out, w)
				}
			}
			if c.gone != "" && strings.Contains(out, c.gone) {
				t.Errorf("said %q, want no %q in it", out, c.gone)
			}
		})
	}
}

// TestTheUsageLineFollowsTheLetterAndNotAlwaysTheName: the two spellings do
// not agree about the usage line, where they agree about everything else a
// refusal reports. One of the panel prints the builtin's usage under a bad
// letter — as it does under any builtin's bad letter — and not under a bad
// `-o` name, whose complaint is a sentence of its own; another prints it
// under both.
func TestTheUsageLineFollowsTheLetterAndNotAlwaysTheName(t *testing.T) {
	usage := Diagnostics{BuiltinUsage: map[string]string{"set": "Usage: set [-abc]"}}
	both := usage
	both.SetInvalidOptionNameUsage = true
	if got := runWorded(t, "set -o bogus\n", usage); strings.Contains(got, "Usage:") {
		t.Errorf("said %q, want no usage line under a refused name for a shell that prints none there", got)
	}
	if got := runWorded(t, "set -o bogus\n", both); !strings.Contains(got, "Usage: set [-abc]") {
		t.Errorf("said %q, want the usage line repeated under the name", got)
	}
	if got := runWorded(t, "set -q\n", usage); !strings.Contains(got, "Usage: set [-abc]") {
		t.Errorf("said %q, want the usage line under the letter either way", got)
	}
}

// TestALetterTheDialectHasIsCalledMissingRatherThanUnknown: telling a script
// that `set -b` is an invalid option, in a dialect that has `-b` and where
// this shell simply does not, would be a different and worse answer than
// telling it the truth. The same rule every other builtin's letters follow —
// see Diagnostics.UnimplementedOptionLetters.
func TestALetterTheDialectHasIsCalledMissingRatherThanUnknown(t *testing.T) {
	dg := Diagnostics{
		SetInvalidOptionLetter:     "set: %[1]s: unknown option",
		UnimplementedOptionLetters: map[string]string{"set": "b"},
		BuiltinUsage:               map[string]string{"set": "Usage: set [-abc]"},
	}
	out := runWorded(t, "set -b\n", dg)
	if !strings.Contains(out, "set: -b is not implemented yet") {
		t.Errorf("said %q, want the letter called missing", out)
	}
	if strings.Contains(out, "unknown option") || strings.Contains(out, "Usage:") {
		t.Errorf("said %q, want neither the dialect's refusal nor its usage line — the shell is not refusing it", out)
	}
	// A letter the dialect does not have either is still refused the
	// dialect's way, so the list narrows the answer rather than replacing it.
	if got := runWorded(t, "set -q\n", dg); !strings.Contains(got, "set: -q: unknown option") {
		t.Errorf("said %q, want the dialect's refusal for a letter it really does not have", got)
	}
}

// An option refused at an *invocation* is not the same sentence as one
// refused by the builtin, and the difference is not only where it happened:
// nothing names `set` there, and a dialect with a usage block of its own
// prints that rather than the builtin's (#598).
//
// Driven through SetOptionLetters and SetNamedOption, which are the front
// end's only way in and therefore what marks the route.
func TestAnOptionRefusedAtAnInvocationDoesNotNameTheBuiltin(t *testing.T) {
	newRunner := func(dg Diagnostics) (*Runner, *strings.Builder) {
		var errs strings.Builder
		sem := PosixSemantics()
		sem.BadSetOptionNameFatal = No
		sem.BadSetOptionLetterFatal = No
		return newTestRunner(t, &Runner{
			Stdout: &strings.Builder{}, Stderr: &errs,
			Semantics: &sem, Diagnostics: &dg, Name: "/opt/x/mysh",
		}), &errs
	}
	t.Run("the sentence loses the builtin's name and the location", func(t *testing.T) {
		r, errs := newRunner(Diagnostics{SetInvalidOptionLetter: "set: Illegal option -%[2]s"})
		r.SetOptionLetters("q", true)
		if got, want := errs.String(), "/opt/x/mysh: Illegal option -q\n"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
	t.Run("a dialect that writes the line it has not reached still does", func(t *testing.T) {
		r, errs := newRunner(Diagnostics{
			SetInvalidOptionLetter:       "set: Illegal option -%[2]s",
			InvocationNamesTheUnreadLine: true,
			Location:                     LocationColonLine,
		})
		r.SetOptionLetters("q", true)
		if got, want := errs.String(), "/opt/x/mysh: 0: Illegal option -q\n"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
	t.Run("the shell's usage block stands in for the builtin's", func(t *testing.T) {
		r, errs := newRunner(Diagnostics{
			BuiltinUsage:    map[string]string{"set": "Usage: set [-abc]"},
			InvocationUsage: "Usage: %[2]s [-abc] — %[1]s",
		})
		r.SetOptionLetters("q", true)
		want := "/opt/x/mysh: -q: invalid option\nUsage: mysh [-abc] — /opt/x/mysh\n"
		if got := errs.String(); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
	t.Run("a dialect that hands the long spelling to the builtin", func(t *testing.T) {
		r, errs := newRunner(Diagnostics{
			InvocationNameRefusalNamesTheShell: true,
			InvocationUsage:                    "Usage: %[2]s [-abc]",
			Location:                           LocationLineWord,
		})
		r.SetNamedOption("bogus", true)
		want := "/opt/x/mysh: line 0: /opt/x/mysh: bogus: invalid option name\n"
		if got := errs.String(); got != want {
			t.Errorf("got %q, want %q — the builtin's own report, with the shell's name where the builtin's would be", got, want)
		}
		// The letter is that dialect's command-line parser speaking and
		// keeps the invocation shape, which is the whole reason the flag
		// asks about the name alone.
		r2, errs2 := newRunner(Diagnostics{
			InvocationNameRefusalNamesTheShell: true,
			InvocationUsage:                    "Usage: %[2]s [-abc]",
			Location:                           LocationLineWord,
		})
		r2.SetOptionLetters("q", true)
		want2 := "/opt/x/mysh: -q: invalid option\nUsage: mysh [-abc]\n"
		if got := errs2.String(); got != want2 {
			t.Errorf("got %q, want %q", got, want2)
		}
	})
	t.Run("and inside a script the builtin is named as it always was", func(t *testing.T) {
		dg := Diagnostics{
			SetInvalidOptionLetter: "set: Illegal option -%[2]s",
			InvocationUsage:        "Usage: %[2]s [-abc]",
		}
		if got := runWorded(t, "set -q\n", dg); !strings.Contains(got, "set: Illegal option -q") ||
			strings.Contains(got, "Usage:") {
			t.Errorf("said %q, want the builtin's own wording and no shell usage block", got)
		}
	})
}

// POSIX mode and the alias-expansion base are independent, and the runner
// holds both so that the front end may write them in either order.
//
// Today it always writes the base first — the route is known before the name
// the shell was invoked under is acted on — so the ordering this pins is not
// reachable through any front end. It is pinned anyway because the guarantee
// is what the two fields are *for*: a base written while the mode is on must
// not turn expansion off, or `sh` would stop expanding aliases the moment
// anything re-read the route.
func TestAliasExpansionAndPosixModeAreIndependent(t *testing.T) {
	// testrunner:bare — the subject is the two option bits and nothing else
	// here reads the filesystem.
	r := &Runner{}
	if r.AliasExpansion() {
		t.Errorf("a fresh runner expands aliases, want off")
	}
	// The mode turns it on over a base that says no.
	r.SetAliasExpansionBase(false)
	r.SetPosixMode(true)
	if !r.AliasExpansion() {
		t.Errorf("posix mode did not turn alias expansion on")
	}
	// And writing the base again while the mode is on leaves it on.
	r.SetAliasExpansionBase(false)
	if !r.AliasExpansion() {
		t.Errorf("re-writing the base turned alias expansion off inside posix mode")
	}
	// Leaving the mode drops to the base, whatever was set in between.
	r.SetAliasExpansion(true)
	r.SetPosixMode(false)
	if r.AliasExpansion() {
		t.Errorf("leaving posix mode kept alias expansion on, want the base back")
	}
	// The other base, and the round trip from it.
	r.SetAliasExpansionBase(true)
	r.SetPosixMode(true)
	r.SetAliasExpansion(false)
	r.SetPosixMode(false)
	if !r.AliasExpansion() {
		t.Errorf("leaving posix mode did not restore a base that was on")
	}
}

// The two editing-mode names are one state, and it has three values.
//
// Measured in bash 5.3 and ksh93 alike, and the third value is the reason a
// bool would not do: `set -o vi` turns `emacs` off in the same breath, and
// `set +o vi` afterwards leaves *both* off rather than putting `emacs` back.
// Turning one on is the only way back to a mode being selected.
//
// A shell with no line to edit has no mode selected either, which is the row
// that changed in #1858 — a keymap is not something a script is in.
func TestTheTwoEditingModesAreOneStateWithThreeValues(t *testing.T) {
	// The listing is what a script reads the state through, and both names
	// are in it, so one script line can show both answers at once.
	for _, tc := range []struct{ name, src, want string }{
		{"a script is in neither mode", "", "emacs off\nvi off"},
		{"vi turns emacs off", "set -o vi\n", "emacs off\nvi on"},
		{"emacs turns vi off", "set -o vi\nset -o emacs\n", "emacs on\nvi off"},
		{"turning vi off leaves neither", "set -o vi\nset +o vi\n", "emacs off\nvi off"},
		{"turning emacs off leaves neither", "set +o emacs\n", "emacs off\nvi off"},
		{
			// Turning off the mode that is not selected moves nothing: the
			// request has already been granted.
			"turning off the other one moves nothing", "set -o vi\nset +o emacs\n",
			"emacs off\nvi on",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src+"set -o\n", withExtras)
			for _, want := range strings.Split(tc.want, "\n") {
				name, state := want[:strings.Index(want, " ")], want[strings.Index(want, " ")+1:]
				if !hasOptionRow(out, name, state) {
					t.Errorf("want %s %s; listing was %q", name, state, out)
				}
			}
		})
	}
	// And neither name is refused any more, in either direction — the
	// refusal is what this replaced.
	for _, src := range []string{"set -o vi", "set +o vi", "set -o emacs", "set +o emacs"} {
		out, _ := run(t, src+"\necho \"st=$?\"\n", withExtras)
		if strings.Contains(out, "not implemented") || !strings.Contains(out, "st=0") {
			t.Errorf("%q said %q, want a granted request", src, out)
		}
	}
}

// hasOptionRow reads one row out of a `set -o` listing without depending on
// how it is padded, since the padding is a dialect's and this is the core's
// test.
func hasOptionRow(listing, name, state string) bool {
	for _, line := range strings.Split(listing, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == name && fields[1] == state {
			return true
		}
	}
	return false
}

// TestARefusalThroughTheDialectsSeamDoesNotEndTheScript: the fatality a
// refused `set` option carries belongs to `set` and not to the option.
//
// `set` is one of the special builtins the standard makes fatal on error, and
// a dialect's own option builtin is an ordinary one. Measured in zsh 5.9.2 on
// a pipe, which is the preset that ends a script over this at all: `set -m`
// writes `can't change option: -m` and the next line never runs, while
// `setopt monitor` — the same option, the same sentence and the same status
// on the builtin — writes `can't change option: monitor`, reports 1, and the
// next line does run.
//
// Asserted from both sides, because a runner that had simply lost the
// fatality would pass the first half alone.
func TestARefusalThroughTheDialectsSeamDoesNotEndTheScript(t *testing.T) {
	fatal := func(r *Runner) {
		sem := CoreSemantics()
		sem.MonitorNeedsATerminal = Yes
		sem.BadSetOptionNameFatal = Yes
		sem.BadSetOptionLetterFatal = Yes
		sem.FatalErrorStatusIsOne = Yes
		r.Semantics = &sem
		r.Diagnostics = &Diagnostics{MonitorDenied: "can't change option: %[1]s", MonitorDeniedStatus: 1}
		r.Register("askoption", func(r *Runner, _ context.Context, _ []string) int {
			return r.ApplyNamedOption("monitor", true)
		})
	}
	out, st := run(t, "askoption\necho \"st=$?\"\necho survived\n", fatal)
	if want := "sh: can't change option: monitor\nst=1\nsurvived\n"; out != want || st != 0 {
		t.Errorf("through the seam: out %q status %d, want %q at 0", out, st, want)
	}
	out, st = run(t, "set -m\necho survived\n", fatal)
	if want := "sh: can't change option: -m\n"; out != want || st != 1 {
		t.Errorf("through `set`: out %q status %d, want %q at 1", out, st, want)
	}
}

// `set -p` is the short spelling of `set -o privileged`, and the letter is
// asked of the dialect: bash, ksh93 and zsh have it and all three mean
// privileged mode by it, where dash and ash refuse it.
//
// Routed through the long name rather than into a field of its own, which is
// what makes the two spellings one question. This shell has no privileged
// mode, so `privileged` is one of the `set -o` entries whose whole answer is
// "already off" — turning it off is granted and turning it on is refused,
// which is setoptions.go's bargain and not a rule about this letter.
func TestThePrivilegedLetter(t *testing.T) {
	has := func(a Answer) func(*Runner) {
		return func(r *Runner) {
			s := *r.Semantics
			s.SetHasThePrivilegedLetter = a
			r.Semantics = &s
			r.AddSetOptions("privileged")
			dg := Diagnostics{}
			r.Diagnostics = &dg
		}
	}

	// The state the shell is already in, asked for by the letter: granted.
	out, st := run(t, `set +p; echo "st=$?"`, has(Yes))
	if !strings.Contains(out, "st=0") {
		t.Errorf("out = %q status %d, want the letter granted", out, st)
	}
	// And by the name, which must give the same answer.
	out, _ = run(t, `set +o privileged; echo "st=$?"`, has(Yes))
	if !strings.Contains(out, "st=0") {
		t.Errorf("out = %q, want the name granted too", out)
	}
	// And the move, which is granted and lands wherever the name lands.
	//
	// It used to be refused here, by the *name* rather than by the letter —
	// and that was the name's answer rather than the letter's then too. The
	// name is recorded since #3128 and the letter followed it without being
	// told, which is the point of routing the letter through the table: two
	// spellings of one question cannot answer differently. Measured
	// 2026-09-16, `set -p` is status 0 in bash 5.3.20, bash 3.2.57 and
	// ksh93u+ alike, and the two shells differ only in where the row lands
	// afterwards — on in bash, off in ksh93, which is
	// Runner.AddInertSetOptions and not the letter's business.
	out, _ = run(t, `set -p; echo "st=$?"`, has(Yes))
	if !strings.Contains(out, "st=0") || strings.Contains(out, "not implemented") {
		t.Errorf("out = %q, want the letter granted through the name", out)
	}
	// And the row it leaves behind, which is the half a status cannot see:
	// recorded here, so the letter really did move the state the name holds.
	out, _ = run(t, "set -p\nset -o\n", has(Yes))
	if got := listedState(out, "privileged"); got != "on" {
		t.Errorf("set -p left the row %q, want on (listing %q)", got, out)
	}
	// Where the dialect declares the name inert the same letter is still
	// granted and the row does not move, which is ksh93's answer.
	out, _ = run(t, "set -p\nset -o\n", func(r *Runner) {
		has(Yes)(r)
		r.AddInertSetOptions("privileged")
	})
	if got := listedState(out, "privileged"); got != "off" {
		t.Errorf("set -p under an inert name left the row %q, want off (listing %q)", got, out)
	}
	// Where the dialect has not got the letter at all, both directions are a
	// bad option rather than a granted no-op.
	out, _ = run(t, `set +p; echo "st=$?"`, has(No))
	if strings.Contains(out, "st=0") {
		t.Errorf("out = %q, want the letter refused where the shell has not got it", out)
	}
}
