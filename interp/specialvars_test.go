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
