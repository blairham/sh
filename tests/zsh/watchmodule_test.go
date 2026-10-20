// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `zsh/watch`: the module a **reference** to either half of `$WATCH`/`$watch`
// loads, and the two parameters it brings with it.
//
// Every `want` is the reference's own answer, measured 2026-09-28 on
// `/opt/homebrew/bin/zsh` — zsh 5.9.2 (aarch64-apple-darwin25.4.0) — run `-f`
// from a script file under `env -i PATH=/usr/bin:/bin TERM=dumb` with a
// scratch `HOME`, one shell per row.

// TestAReferenceToTheWatchPairLoadsItsModule is the row #4998 filed, with the
// trigger corrected: the issue reported a **write**, and it is a *reference*.
//
// **The probe names neither `WATCHFMT` nor `LOGCHECK`**, and that is not
// fastidiousness: a probe that asked `${+WATCHFMT}` would be asking the module
// to arrive in order to find out whether it had. `zmodload -e` asks the
// module directly and touches no parameter.
//
// The last four rows are the control and they are what makes this a rule about
// a reference rather than about the name appearing anywhere — a listing and an
// `unset` mention `WATCH` and load nothing, and the module's *other* two names
// load nothing either.
func TestAReferenceToTheWatchPairLoadsItsModule(t *testing.T) {
	const probe = "\nzmodload -e zsh/watch; print -r -- \"loaded=$?\""
	for _, tc := range []struct {
		name, src string
		loaded    bool
	}{
		{"nothing at all", ":", false},
		{"a set test on the array half", ": ${+watch}", true},
		{"a set test on the scalar half", ": ${+WATCH}", true},
		{"a plain read", `: "$watch"`, true},
		{"a write to the array half", "watch=(a b)", true},
		{"a write to the scalar half", "WATCH=cc", true},
		// The controls.
		{"the module's own other name", ": ${+WATCHFMT}", false},
		{"and its second", ": ${+LOGCHECK}", false},
		{"a listing", "typeset -p WATCH >/dev/null 2>&1", false},
		{"an unset", "unset watch", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := "loaded=1\n"
			if tc.loaded {
				want = "loaded=0\n"
			}
			out, st := runZsh(t, t.TempDir(), tc.src+probe)
			if out != want || st != 0 {
				t.Errorf("= %q (status %d), want %q", out, st, want)
			}
		})
	}
}

// And what the module brings, by either route.
//
// Both parameters are **plain**: no `special`, neither hiding letter, and both
// writable. So what is modeled is the arrival and the values — there is no
// login watch in this shell for them to configure.
func TestTheWatchModuleBringsItsTwoParameters(t *testing.T) {
	const probe = "\nprint -r -- \"+WF=${+WATCHFMT} +LC=${+LOGCHECK} " +
		"WF=[$WATCHFMT] LC=[$LOGCHECK] tWF=[${(t)WATCHFMT}] tLC=[${(t)LOGCHECK}]\""
	const want = "+WF=1 +LC=1 WF=[%n has %a %l from %m.] LC=[60] " +
		"tWF=[scalar] tLC=[integer]\n"
	for _, tc := range []struct{ name, src string }{
		{"through a reference to the pair", "watch=(a b)"},
		{"through an explicit load", "zmodload zsh/watch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src+probe)
			if out != want || st != 0 {
				t.Errorf("= %q (status %d), want %q", out, st, want)
			}
		})
	}
	// The control: neither name exists in a shell that has done neither.
	t.Run("and neither before either", func(t *testing.T) {
		out, st := runZsh(t, t.TempDir(),
			`print -r -- "+WF=${+WATCHFMT} +LC=${+LOGCHECK}"`)
		if out != "+WF=0 +LC=0\n" || st != 0 {
			t.Errorf("= %q (status %d), want both absent", out, st)
		}
	})
}

// The listing and the writes, which say the two are ordinary parameters
// rather than views of anything.
func TestTheWatchModulesParametersAreOrdinary(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the listing", "zmodload zsh/watch\ntypeset -p WATCHFMT LOGCHECK",
			"typeset WATCHFMT='%n has %a %l from %m.'\ntypeset -i LOGCHECK=60\n",
		},
		{
			"a write to each", "zmodload zsh/watch\nWATCHFMT=x; LOGCHECK=5\n" +
				`print -r -- "st=$? WF=[$WATCHFMT] LC=[$LOGCHECK]"`,
			"st=0 WF=[x] LC=[5]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("= %q (status %d), want %q", out, st, tc.want)
			}
		})
	}
}

// A second `zmodload zsh/watch` is a silent 0, which is the row that says the
// feature gate reads the **mark** and not the deferral: the first load refers
// to the module's own parameters, so a gate keyed on "is this name still
// waiting" says yes once and no afterwards.
func TestLoadingTheWatchModuleTwiceIsSilent(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		"zmodload zsh/watch\nzmodload zsh/watch\nprint -r -- \"st=$?\"")
	if out != "st=0\n" || st != 0 {
		t.Errorf("= %q (status %d), want st=0", out, st)
	}
}
