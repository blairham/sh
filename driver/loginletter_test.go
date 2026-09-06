// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

// Login-ness is the other startup input `$-` reports, and it is the front
// end's to carry in: `interp` never saw an argument vector, and no `set`
// letter turns it on for the option table to have written (#1034).
//
// Membership rather than the string, for the reason routeMembership uses it:
// no two shells in the panel order `$-` alike.
const loginMembership = `case $- in *l*) echo has-l ;; *) echo no-l ;; esac`

// loginShell answers the login axis and gives the dialect the option
// spellings, so a test can say which answer it is exercising without naming a
// shell. HOME is a scratch directory: a login shell reads profiles, and a
// test that reads the person's own is a test whose answer is their machine's.
func loginShell(t *testing.T, shows interp.Answer) driver.Shell {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	sh := shell()
	sh.Semantics.LoginShowsLInDollarDash = shows
	sh.Semantics.StartupFileOptions = interp.StartupFileOptions{Login: "-l --login"}
	return sh
}

// TestTheLoginFactReachesDollarDash: the fact arrives by either route there
// is — an explicit option or a dashed argv[0] — and the axis decides whether
// the letter is written.
func TestTheLoginFactReachesDollarDash(t *testing.T) {
	script := writeScript(t, loginMembership+"\n")
	for _, tc := range []struct {
		name  string
		shows interp.Answer
		argv  []string
		want  string
	}{
		{
			"the letters bundled, in a shell that shows it",
			interp.Yes,
			[]string{"testsh", "-lc", loginMembership},
			"has-l\n",
		},
		{
			"the letters bundled, in a shell that keeps the fact elsewhere",
			interp.No,
			[]string{"testsh", "-lc", loginMembership},
			"no-l\n",
		},
		{
			// The pair the corpus asks as its own case: without it a front
			// end that recorded the fact only for a bundle would pass above.
			"the same letters as their own words",
			interp.Yes,
			[]string{"testsh", "-l", "-c", loginMembership},
			"has-l\n",
		},
		{
			"the long spelling",
			interp.Yes,
			[]string{"testsh", "--login", "-c", loginMembership},
			"has-l\n",
		},
		{
			// The route no option can reach, and what `login` and every
			// terminal emulator's "run as a login shell" actually does.
			"a dashed argv[0] with no option written at all",
			interp.Yes,
			[]string{"-testsh", "-c", loginMembership},
			"has-l\n",
		},
		{
			// The control that keeps the row above honest: a front end
			// writing the letter unconditionally would pass it.
			"an undashed argv[0] is no login shell",
			interp.Yes,
			[]string{"testsh", "-c", loginMembership},
			"no-l\n",
		},
		{
			// Login-ness is not a route, so it survives every one of them.
			"a script operand is still a login shell",
			interp.Yes,
			[]string{"testsh", "-l", script},
			"has-l\n",
		},
		{
			"a script operand with no login-ness anywhere",
			interp.Yes,
			[]string{"testsh", script},
			"no-l\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, code := runArgs(t, loginShell(t, tc.shows), tc.argv...)
			if code != 0 {
				t.Fatalf("status %d, stderr %q", code, errs)
			}
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// TestTheLoginLetterIsReadAndNotAsked: an unanswered axis shows no letter and
// says nothing, the way the two `$-` route axes beside it do. Refusing a whole
// `$-` expansion over it would break `case $- in *e*)` — the ordinary errexit
// check — in every script running under a preset that has not chosen.
func TestTheLoginLetterIsReadAndNotAsked(t *testing.T) {
	sh := loginShell(t, interp.Unspecified)
	var o, e bytes.Buffer
	sh.Stdout, sh.Stderr = &o, &e
	if code := driver.MainArgs(sh, []string{"testsh", "-lc", loginMembership}); code != 0 {
		t.Fatalf("status %d, stderr %q", code, e.String())
	}
	if got, want := o.String(), "no-l\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if s := e.String(); s != "" {
		t.Errorf("stderr = %q, want the letter decided without a word said", s)
	}
	if strings.Contains(e.String(), "disagree") {
		t.Errorf("stderr = %q, want no unanswered-axis refusal", e.String())
	}
}

// TestTheLoginLetterJoinsTheOthers: `$-` is one string, and the letter takes
// its place in it beside the ones a `set` option and another startup fact
// write. Membership of the whole set, because the order is ours and no
// script may depend on it.
func TestTheLoginLetterJoinsTheOthers(t *testing.T) {
	sh := loginShell(t, interp.Yes)
	sh.Semantics.CommandStringShowsCInDollarDash = interp.Yes
	out, errs, code := runArgs(t, sh, "testsh", "-lec", `echo "[$-]"`)
	if code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs)
	}
	got := strings.TrimSpace(out)
	got = strings.TrimPrefix(strings.TrimSuffix(got, "]"), "[")
	for _, letter := range []string{"l", "e", "c"} {
		if !strings.Contains(got, letter) {
			t.Errorf("$- = %q, want %q among the letters", got, letter)
		}
	}
}
