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
//
// `posix` was on this list until it became a mode this shell really has —
// which is the shape of the rule rather than an exception to it: the promise
// can be made now, so the request is granted. TestPosixModeMovesAnAxis is
// where it is held to it. `vi` left the list the same way, and
// TestTheTwoEditingModesAreOneStateWithThreeValues is where it is held to it.
func TestTurningOnWhatThisShellDoesNotDoIsRefused(t *testing.T) {
	for _, name := range []string{"notify"} {
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
			Diagnostics{SetInvalidOptionName: "set: no such option: %[1]s", SetInvalidOptionStatus: 1},
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
	// Not fatal here, so that what `set` reported can still be printed.
	sem.BadSetOptionNameFatal = No
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
	if !strings.Contains(out, "set: -T: invalid option") || !strings.Contains(out, "st=2") {
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
		name string
		dg   Diagnostics
		want string
	}{
		{"a shell with a status of its own", Diagnostics{SetInvalidOptionStatus: 1}, "st=1"},
		{"and the default", Diagnostics{}, "st=2"},
	} {
		t.Run(c.name, func(t *testing.T) {
			// The same value read through both spellings, in one run, so
			// that a change teaching one of them and not the other fails.
			out := runWorded(t, "set -q\necho \"st=$?\"\nset -o bogus\necho \"st=$?\"\n", c.dg)
			if got := strings.Count(out, c.want); got != 2 {
				t.Errorf("said %q, want %q twice — the letter and the name report the same status, got it %d time(s)", out, c.want, got)
			}
		})
	}
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
			sem.BadSetOptionNameFatal = c.fatal
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
		sem.BadSetOptionNameFatal = No
		dg := Diagnostics{SetInvalidOptionStatus: status}
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
func TestTheTwoEditingModesAreOneStateWithThreeValues(t *testing.T) {
	// The listing is what a script reads the state through, and both names
	// are in it, so one script line can show both answers at once.
	for _, tc := range []struct{ name, src, want string }{
		{"a fresh shell is in emacs mode", "", "emacs on\nvi off"},
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
