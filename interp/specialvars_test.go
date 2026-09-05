// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The parameters the substrate provides itself, which are the three the panel
// agrees on. This names the behavior rather than a shell, as the rule for
// this package requires; which dialect has UID or RANDOM is asserted in
// dialect/.
func TestTheSubstrateProvidesTheUnanimousParameters(t *testing.T) {
	// IFS is the one that looked least urgent and mattered most: splitting
	// already used a default when it was unset, so everything *worked* while
	// a script could neither read it nor tell it had been changed.
	if out, _ := run(t, `printf '%s' "$IFS"`, nil); out != " \t\n" {
		t.Errorf("IFS = %q, want space, tab and newline", out)
	}
	if out, _ := run(t, `[ -n "$PPID" ] && echo have`, nil); strings.TrimSpace(out) != "have" {
		t.Errorf("PPID: got %q", out)
	}
	// Produced when read: a stored copy would be the line the shell started
	// on, so the two lines here would print the same number.
	out, _ := run(t, "echo \"$LINENO\"\necho \"$LINENO\"", nil)
	if out != "1\n2\n" {
		t.Errorf("LINENO gave %q, want each line to report itself", out)
	}
}

// A produced parameter cannot be overwritten by assigning to it, and what was
// assigned is kept where its producer can see it.
func TestAssigningAProducedParameterReachesItsProducer(t *testing.T) {
	setup := func(r *Runner) {
		r.SetDynamic("COUNTER", func(rr *Runner) string {
			if v, ok := rr.Assigned("COUNTER"); ok {
				return "from:" + v
			}
			return "produced"
		})
	}
	if out, _ := run(t, `echo "$COUNTER"`, setup); strings.TrimSpace(out) != "produced" {
		t.Errorf("got %q, want the produced value", out)
	}
	// The assignment does not shadow the producer — it reaches it.
	if out, _ := run(t, `COUNTER=7; echo "$COUNTER"`, setup); strings.TrimSpace(out) != "from:7" {
		t.Errorf("got %q, want the producer to have seen the assignment", out)
	}
	// And `unset` still takes it away, ahead of both.
	if out, _ := run(t, `unset COUNTER; echo "[${COUNTER-gone}]"`, setup); strings.TrimSpace(out) != "[gone]" {
		t.Errorf("got %q, want it removed", out)
	}
}

// `$-` is the set of single-letter options in effect, produced when it is
// read: a letter appears while `set -` has its option on and is gone once
// `set +` takes it off. While this expanded to nothing, `case $- in *e*)` —
// the standard errexit check — silently took the wrong branch.
func TestDollarDashReflectsTheLiveOptionState(t *testing.T) {
	sem := CoreSemantics()
	setup := func(r *Runner) { r.Semantics = &sem }

	// The idiom the parameter exists for.
	if out, _ := run(t, `set -e; case $- in *e*) echo has-e;; *) echo no-e;; esac`, setup); out != "has-e\n" {
		t.Errorf("errexit check got %q, want has-e", out)
	}
	// Each tracked option contributes its letter, and turning one off takes
	// exactly that letter away. Order within our own letters is fixed;
	// presence is the contract.
	out, _ := run(t, `set -a; set -e; set -u; set -C; echo "[$-]"; set +e; echo "[$-]"; set +a; set +u; set +C; echo "[$-]"`, setup)
	if out != "[aeuC]\n[auC]\n[]\n" {
		t.Errorf("got %q, want the letters to track the options", out)
	}
	// xtrace's letter, asserted by presence because the trace itself shares
	// the buffer.
	if out, _ := run(t, `set -x; case $- in *x*) echo has-x;; *) echo no-x;; esac`, nil); !strings.Contains(out, "has-x") || strings.Contains(out, "no-x") {
		t.Errorf("xtrace check got %q, want has-x", out)
	}
	// pipefail earns no letter, which is unanimous across the panel.
	psem := CoreSemantics()
	psem.PipefailOption = Yes
	if out, _ := run(t, `set -o pipefail; echo "[$-]"`, func(r *Runner) { r.Semantics = &psem }); out != "[]\n" {
		t.Errorf("pipefail got %q, want no letter", out)
	}
}

// The letters a shell turns on at startup are the dialect's to declare, and
// the substrate prefixes whatever it was given — the value here is made up,
// because which real shell reports what is asserted in dialect/.
func TestDollarDashStartsWithTheDialectsDefaultLetters(t *testing.T) {
	sem := CoreSemantics()
	sem.DefaultOptionLetters = "789Z"
	out, _ := run(t, `echo "[$-]"; set -e; echo "[$-]"`, func(r *Runner) { r.Semantics = &sem })
	if out != "[789Z]\n[789Ze]\n" {
		t.Errorf("got %q, want the default letters first and the live ones after", out)
	}
}

// `i` is in `$-` when the front end said the shell is interactive, and only
// then. No axis: the panel is unanimous both ways, and there is no `set`
// letter that turns it on, so the fact has to arrive from outside.
//
// It arrives on the Runner rather than being discovered, which is the rule
// this package lives by: a library has no standing to ask the process whether
// anybody is watching, and the process would answer about the *embedder's*
// invocation rather than about this shell's.
func TestDollarDashSaysInteractiveOnlyWhenItWasToldSo(t *testing.T) {
	const src = `echo "[$-]"; case $- in *i*) echo yes ;; *) echo no ;; esac`
	for _, tc := range []struct {
		name        string
		interactive bool
		want        string
	}{
		{"told so", true, "[i]\nyes\n"},
		{"not told", false, "[]\nno\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := CoreSemantics()
			out, _ := run(t, src, func(r *Runner) {
				r.Semantics, r.Interactive = &sem, tc.interactive
			})
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}

	// And it is not a `set` option, so neither sign of it is a letter the
	// option table knows: an interactive shell cannot be turned into a
	// non-interactive one halfway through, which is unanimous.
	out, _ := run(t, `set +i 2>/dev/null; case $- in *i*) echo yes ;; *) echo no ;; esac`,
		func(r *Runner) { r.Interactive = true })
	if !strings.Contains(out, "yes") {
		t.Errorf("got %q, want `set +i` to leave the shell interactive", out)
	}
}

// Which letter noglob shows is an axis: POSIX names `f`, and one shell in the
// panel reports the capital because that is its own short spelling.
func TestNoglobLetterFollowsTheAxis(t *testing.T) {
	for _, tc := range []struct {
		name string
		isF  Answer
		want string
	}{
		{"lowercase", Yes, "[f]\n"},
		{"uppercase", No, "[F]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := CoreSemantics()
			sem.NoglobLetterIsF = tc.isF
			out, _ := run(t, `set -o noglob; echo "[$-]"`, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// A parameter a dialect does not provide stays absent, which is what makes
// `${RANDOM-}` a usable test for having it.
func TestAnUnprovidedParameterIsAbsent(t *testing.T) {
	if out, _ := run(t, `[ -n "${NOSUCHPARAM-}" ] && echo have || echo none`, nil); strings.TrimSpace(out) != "none" {
		t.Errorf("got %q, want none", out)
	}
}

// TestUnderscoreFollowsTheLastArgumentIsAnAxis — bash and zsh move $_ to the
// previous simple command's last expanded argument; two shells never touch it.
func TestUnderscoreFollowsTheLastArgumentIsAnAxis(t *testing.T) {
	track := func(r *Runner) {
		sem := CoreSemantics()
		sem.UnderscoreTracksTheLastArgument = Yes
		r.Semantics = &sem
	}
	out, _ := run(t, `echo one two >/dev/null; echo "[$_]"`, track)
	if !strings.Contains(out, "[two]") {
		t.Errorf("out=%q, want the last argument", out)
	}
	out, _ = run(t, `echo hi >/dev/null; x=5; echo "[$_]"`, track)
	if !strings.Contains(out, "[]") {
		t.Errorf("out=%q, want empty after a bare assignment", out)
	}
	out, _ = run(t, `w=expanded; echo "$w" >/dev/null; echo "[$_]"`, track)
	if !strings.Contains(out, "[expanded]") {
		t.Errorf("out=%q, want the expanded argument, not the written one", out)
	}
	out, _ = run(t, `echo one >/dev/null; echo "[$_]"`, func(r *Runner) {
		sem := CoreSemantics()
		sem.UnderscoreTracksTheLastArgument = No
		r.Semantics = &sem
	})
	if strings.Contains(out, "[one]") {
		t.Errorf("out=%q, want the axis off to leave $_ alone", out)
	}
}

// The route letters, which arrive on the Runner the way `i` does and for the
// same reason: where the program came from is a fact about the invocation,
// and a library Runner would otherwise read the embedder's command line.
//
// `s` for the standard-input route is unanimous across the panel and so has
// no axis. `c` for a command string is two against two, and `s` under `-c` is
// one shell against three; both are named axes here rather than shells, as
// this package requires (#551).
func TestDollarDashShowsTheRouteItWasInvokedBy(t *testing.T) {
	const src = `case $- in *c*) echo has-c ;; *) echo no-c ;; esac
case $- in *s*) echo has-s ;; *) echo no-s ;; esac`
	for _, tc := range []struct {
		name     string
		route    Route
		stdinOpt bool
		showsC   Answer
		showsS   Answer
		want     string
	}{
		{
			"a script file shows neither, whatever the axes say",
			RouteScriptFile, false, Yes, Yes, "no-c\nno-s\n",
		},
		{
			"standard input shows `s` with no axis asked",
			RouteStandardInput, false, Unspecified, Unspecified, "no-c\nhas-s\n",
		},
		{
			"a command string, in a shell that shows the letter",
			RouteCommandString, false, Yes, No, "has-c\nno-s\n",
		},
		{
			"a command string, in a shell that does not",
			RouteCommandString, false, No, No, "no-c\nno-s\n",
		},
		{
			"a command string, in the shell that also shows `s` there",
			RouteCommandString, false, Yes, Yes, "has-c\nhas-s\n",
		},
		{
			"`-s` written, and a command string supplying the program anyway",
			RouteCommandString, true, No, No, "no-c\nhas-s\n",
		},
		{
			"a Runner nobody told, which is neither route",
			RouteUnspecified, false, Yes, Yes, "no-c\nno-s\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := CoreSemantics()
			sem.CommandStringShowsCInDollarDash = tc.showsC
			sem.CommandStringShowsSInDollarDash = tc.showsS
			out, _ := run(t, src, func(r *Runner) {
				r.Semantics = &sem
				r.Route, r.StandardInputOption = tc.route, tc.stdinOpt
			})
			if out != tc.want {
				t.Errorf("%v: got %q, want %q", tc.route, out, tc.want)
			}
		})
	}
}

// TestAnUnansweredRouteLetterIsSilent: both route axes are read rather than
// asked, which is the one place in this package that matters. `case $- in
// *e*)` is the ordinary errexit check and it runs in scripts that have chosen
// no dialect; refusing the whole expansion over a letter nobody asked for
// would break every one of them.
func TestAnUnansweredRouteLetterIsSilent(t *testing.T) {
	sem := CoreSemantics()
	out, st := run(t, `set -e; case $- in *e*) echo has-e ;; *) echo no-e ;; esac`,
		func(r *Runner) { r.Semantics, r.Route = &sem, RouteCommandString })
	if out != "has-e\n" || st != 0 {
		t.Errorf("got %q at %d, want %q at 0 with nothing said about the unchosen route letters", out, st, "has-e\n")
	}
}

// TestDollarDashTakesTheInteractiveLettersInsteadOfTheDefaultOnes.
//
// The second startup vector, and the reason it is a replacement rather than a
// list of letters to add: one shell in the panel turns an option *off* when it
// is interactive, so what an interactive shell starts with is a different set
// and not a longer one. The letters here are made up, because which real shell
// reports what is asserted in dialect/.
func TestDollarDashTakesTheInteractiveLettersInsteadOfTheDefaultOnes(t *testing.T) {
	sem := CoreSemantics()
	sem.DefaultOptionLetters = "789Z"
	// A set that keeps one of the four, drops three and adds one — which no
	// "letters to add" field could express.
	sem.InteractiveOptionLetters = "7Q"
	for _, tc := range []struct {
		name        string
		interactive bool
		want        string
	}{
		{"interactive", true, "[7Qi]\n"},
		{"not", false, "[789Z]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, `echo "[$-]"`, func(r *Runner) {
				r.Semantics = &sem
				r.Interactive = tc.interactive
			})
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// TestAnUnansweredInteractiveVectorLeavesTheDefaultLettersStanding, which is
// one of the four panel members' real answer and the zero value's meaning:
// a shell whose `$-` is the same set either way says nothing here.
func TestAnUnansweredInteractiveVectorLeavesTheDefaultLettersStanding(t *testing.T) {
	sem := CoreSemantics()
	sem.DefaultOptionLetters = "789Z"
	out, _ := run(t, `echo "[$-]"`, func(r *Runner) {
		r.Semantics = &sem
		r.Interactive = true
	})
	if want := "[789Zi]\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// TestTheInteractiveLettersAreStillOnlyTheStartingPoint. A `set` after startup
// adds to whichever of the two vectors was chosen, exactly as it adds to the
// default one — the vector says where `$-` begins and never what it ends as.
func TestTheInteractiveLettersAreStillOnlyTheStartingPoint(t *testing.T) {
	sem := CoreSemantics()
	sem.DefaultOptionLetters = "789Z"
	sem.InteractiveOptionLetters = "7Q"
	out, _ := run(t, `set -e; echo "[$-]"`, func(r *Runner) {
		r.Semantics = &sem
		r.Interactive = true
	})
	if want := "[7Qie]\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// TestTheMonitorLetterComesFromTheMonitorAndNotFromTheVector.
//
// One shell in the panel shows `m` only when it is interactive, and it shows it
// because job control is really on there. Writing the letter into the
// interactive vector would report a monitor that is not running, so the vector
// must not carry it and the letter has to keep coming from the runner's own
// state — asserted from both sides here, since a vector that quietly carried it
// would pass the first half alone.
func TestTheMonitorLetterComesFromTheMonitorAndNotFromTheVector(t *testing.T) {
	sem := CoreSemantics()
	sem.InteractiveOptionLetters = "Q"
	src := `case $- in *m*) echo has-m ;; *) echo no-m ;; esac`
	out, _ := run(t, src, func(r *Runner) {
		r.Semantics = &sem
		r.Interactive = true
	})
	if want := "no-m\n"; out != want {
		t.Errorf("interactive with no monitor: got %q, want %q", out, want)
	}
	out, _ = run(t, "set -m\n"+src, func(r *Runner) {
		r.Semantics = &sem
		r.Interactive = true
		r.JobControl = true
	})
	if want := "has-m\n"; out != want {
		t.Errorf("with the monitor really on: got %q, want %q", out, want)
	}
}
