// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// The `set -o` roster, by byte.
//
// Measured 2026-09-15 against ksh93u+ 2012-08-01 by diffing the whole listing.
// It is thirty-two rows there, it was twenty here, and five of the twenty were
// spelled the other way round: this shell lists `clobber`, `exec`, `glob`,
// `log` and `unset` — each `on` in a stock shell — where the other four
// columns list `noclobber`, `noexec`, `noglob`, `nolog` and `nounset`, each
// off. Same state, opposite row (#2925).
//
// A listing is a capture surface: a script saves and restores option state
// with `eval "$(set +o)"`, so a name missing from it is a state that save
// cannot carry. `multiline` and `viraw` are the two that were both on and
// absent.
//
// Whole and ordered rather than a contains-check per name, because the
// omissions are what the case is about and nothing that tests only for
// presence can see one.
func TestTheOptionListingIsTheWholeRoster(t *testing.T) {
	out, st := answersRun(t, "set -o\n")
	want := "Current option settings\n" + strings.Join([]string{
		"allexport                off",
		"bgnice                   off",
		"braceexpand              on",
		"clobber                  on",
		"emacs                    off",
		"errexit                  off",
		"exec                     on",
		"glob                     on",
		"globstar                 off",
		"gmacs                    off",
		"histexpand               off",
		"ignoreeof                off",
		"interactive              off",
		"keyword                  off",
		"letoctal                 off",
		"log                      on",
		"login_shell              off",
		"markdirs                 off",
		"monitor                  off",
		"multiline                on",
		"notify                   off",
		"pipefail                 off",
		"privileged               off",
		"rc                       off",
		"restricted               off",
		"showme                   off",
		"trackall                 on",
		"unset                    on",
		"verbose                  off",
		"vi                       off",
		"viraw                    on",
		"xtrace                   off",
		"",
	}, "\n")
	if out != want || st != 0 {
		t.Errorf("set -o:\n got %q at %d\nwant %q at 0", out, st, want)
	}
}

// The five negated rows follow the state they negate, through either
// spelling. `noclobber` is still a name this shell **takes** — only the
// roster changed — which is measured: `set -o noclobber` there is status 0
// and the next listing says `clobber off`.
func TestTheNegatedRowsFollowTheStateEitherWay(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"set -o noclobber\nset -o\n", "clobber                  off"},
		{"set +o clobber\nset -o\n", "clobber                  off"},
		{"set -o nounset\nset -o\n", "unset                    off"},
		{"set +o unset\nset -o\n", "unset                    off"},
		{"set -o noglob\nset -o\n", "glob                     off"},
		{"set +o glob\nset -o\n", "glob                     off"},
	} {
		out, st := answersRun(t, tc.src)
		if st != 0 || !strings.Contains(out, tc.want+"\n") {
			t.Errorf("%q:\n got %q at %d\nwant a %q row at 0", tc.src, out, st, tc.want)
		}
		// And the substrate's own spelling is in none of them.
		for _, line := range strings.Split(out, "\n") {
			if strings.HasPrefix(line, "no") && !strings.HasPrefix(line, "notify") {
				t.Errorf("%q: listing has %q, which this shell does not write", tc.src, line)
			}
		}
	}
}

// `set +o` here names the states that are **stored** rather than the options
// that are on, which is why the negated rows appear under the substrate's
// spelling and only when they are off. Measured, and byte for byte:
//
//	$ ksh -c 'set +o'
//	set --default --braceexpand --multiline --trackall --viraw
//	$ ksh -c 'set +o clobber; set +o'
//	set --default --braceexpand --noclobber --multiline --trackall --viraw
//	$ ksh -c 'set +o unset; set +o'
//	set --default --braceexpand --multiline --trackall --nounset --viraw
//
// The negated spelling lands where the *listed* name sorts, which is what the
// last of those three says: `--noclobber` comes after `--braceexpand` and
// `--nounset` after `--trackall`.
func TestThePlusOLineNamesTheStoredStates(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"set +o\n", "set --default --braceexpand --multiline --trackall --viraw\n"},
		{
			"set +o clobber\nset +o\n",
			"set --default --braceexpand --noclobber --multiline --trackall --viraw\n",
		},
		{
			"set -o nounset\nset +o\n",
			"set --default --braceexpand --multiline --trackall --nounset --viraw\n",
		},
		{
			"set -o errexit\nset +o\n",
			"set --default --braceexpand --errexit --multiline --trackall --viraw\n",
		},
		{
			"set +o trackall\nset +o\n",
			"set --default --braceexpand --multiline --viraw\n",
		},
	} {
		out, st := answersRun(t, tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%q:\n got %q at %d\nwant %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// Three of the rows are listed and refused. `set -o interactive`,
// `set -o login_shell` and `set -o rc` are `bad option(s)` in **both**
// directions here, word for word with a name this shell has never heard of,
// and they are left out of the `set +o` line because that line is a command.
func TestTheInvocationRowsAreListedAndRefused(t *testing.T) {
	for _, name := range []string{"interactive", "login_shell", "rc"} {
		for _, sign := range []string{"-", "+"} {
			src := "set " + sign + "o " + name + "\nprintf 'reached\\n'\n"
			out, st := answersRun(t, src)
			want := "sh: set: " + name + ": bad option(s)\n"
			if !strings.HasPrefix(out, want) || strings.Contains(out, "reached") {
				t.Errorf("%q:\n got %q at %d\nwant it to open %q and stop", src, out, st, want)
			}
		}
	}
	// And the same three names are in the listing all the same.
	out, _ := answersRun(t, "set -o\n")
	for _, row := range []string{"interactive              off", "login_shell              off", "rc                       off"} {
		if !strings.Contains(out, row+"\n") {
			t.Errorf("set -o = %q, want a %q row", out, row)
		}
	}
}
