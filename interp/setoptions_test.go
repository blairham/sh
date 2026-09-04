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
	for _, name := range []string{"posix", "noexec", "verbose", "notify", "vi", "hashall"} {
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
	for _, name := range []string{"posix", "noexec", "verbose", "notify", "vi", "hashall"} {
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
