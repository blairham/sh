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

// And turning one *on* is refused, because accepting would be promising to
// behave differently afterwards.
func TestTurningOnWhatThisShellDoesNotDoIsRefused(t *testing.T) {
	for _, name := range []string{"posix", "notify", "vi"} {
		t.Run(name, func(t *testing.T) {
			// The status of `set` itself, which a later command would
			// otherwise replace — the first version of this test asserted
			// the script's status and passed on the strength of the `echo`
			// after it.
			out, _ := run(t, "set -o "+name+"\necho \"st=$?\"\n", withExtras)
			if !strings.Contains(out, "not implemented") {
				t.Errorf("set -o %s said %q, want it refused as unimplemented", name, out)
			}
			if !strings.Contains(out, "st=2") {
				t.Errorf("set -o %s said %q, want status 2", name, out)
			}
		})
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

// A name we are already doing goes the other way round: it can be turned on
// and not off. Braces are expanded here, so a script may ask for that and may
// not ask us to stop.
func TestANameThisShellAlreadyDoesTurnsOnAndNotOff(t *testing.T) {
	if out, st := run(t, "set -o braceexpand\necho {a,b}\n", withExtras); st != 0 || !strings.Contains(out, "a b") {
		t.Errorf("set -o braceexpand gave %q (status %d), want it accepted", out, st)
	}
	out, _ := run(t, "set +o braceexpand\necho \"st=$?\"\n", withExtras)
	if !strings.Contains(out, "not implemented") || !strings.Contains(out, "st=2") {
		t.Errorf("set +o braceexpand gave %q, want it refused — braces are expanded here", out)
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
	r.AddSetOptions("braceexpand", "hashall", "posix", "privileged")
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
			Diagnostics{BuiltinUsage: map[string]string{"set": "Usage: set [-abc]"}},
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
	// Not fatal here, so that what `set` reported can still be printed.
	sem.BadSetOptionNameFatal = No
	r := &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "sh"}
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
// -T carries DEBUG and RETURN; only the dialect with the letters takes them.
func TestSetTraceLettersAreAnAxis(t *testing.T) {
	withE := func(r *Runner) {
		sem := CoreSemantics()
		sem.SetHasTraceLetters = Yes
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
		sem.SetHasTraceLetters = No
		r.Semantics = &sem
	})
	if !strings.Contains(out, "not implemented") || !strings.Contains(out, "st=2") {
		t.Errorf("out=%q, want the dialect without the letters to refuse at 2", out)
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
		sem.BadSetOptionNameFatal = No
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
	// A front end that gave this runner a person to report jobs to has a
	// terminal, and the question is never asked.
	out, st = run(t, `set -m; echo "st=$?"`, func(r *Runner) {
		remarks(r)
		r.JobControl = true
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
	if !strings.Contains(out, "not implemented") || !strings.Contains(out, "st=2") {
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
