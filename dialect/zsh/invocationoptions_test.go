// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"testing"

	"github.com/blairham/sh/driver"
)

// The four names beside `singlecommand` that a running script cannot move and
// the command line can.
//
// zsh refuses exactly five — `interactive`, `monitor`, `shinstdin`,
// `singlecommand` and `zle` — and takes four of the five at an invocation.
// `singlecommand` was the one already built here and is the worked example
// the rest follow; the refusals a script gets were right all along, and the
// route is what was missing (#3154).
//
// Measured 2026-09-16 on zsh 5.9.2 with `-f`, which this shell does not need
// because it reads no startup files in a test.
//
// **The suite is what covers this and these rows are the second reading.**
// The grant lives where the front end hands a `-o name` or a `--name` word to
// the option table, and a test that called the resolver would pass with that
// routing gone — which is exactly what #3129 recorded when
// `LongOptionNamesASetOption` was dropped and every Go test stayed green. So
// these go through driver.MainArgs, the way the suite rows do.
func TestTheInvocationTakesTheFourOptionsAScriptCannotMove(t *testing.T) {
	for _, tc := range []struct {
		name string
		argv []string
		want string
	}{
		{
			// Interactive, so `$-` gains `i` — and `Z` with it, because
			// `zle` is read over the same state and an interactive shell has
			// an editor. Real zsh answers `569XZfi` here against `569Xf`
			// with nothing asked.
			"interactive", []string{"zsh", "-o", "interactive", "-c", `echo "[$-]"`}, "[569XZi]\n",
		},
		{
			"the long spelling", []string{"zsh", "--interactive", "-c", `echo "[$-]"`}, "[569XZi]\n",
		},
		{
			// The state behind the `s` this shell already wrote on the route
			// where the program really does arrive on standard input.
			"shinstdin", []string{"zsh", "-o", "shinstdin", "-c", `echo "[$-]"`}, "[569Xs]\n",
		},
		{
			// Granted and **inert**: status 0, nothing said, and the option
			// still off afterwards. Neither the refusal a script gets nor a
			// move, which is measured — `zsh -f -o monitor -c '[[ -o monitor ]]'`
			// is 0 for the request and 1 for the read.
			"monitor", []string{"zsh", "-o", "monitor", "-c", `[[ -o monitor ]] && echo on; echo asked`}, "asked\n",
		},
		{
			"zle", []string{"zsh", "-o", "zle", "-c", `[[ -o zle ]] && echo on; echo asked`}, "asked\n",
		},
		{
			// And `+o zle` is the row that says the last of those is about
			// the route rather than the name: with `interactive` ahead of it
			// the option really is on, and turning it off takes the `Z` back.
			"the editor turned off",
			[]string{"zsh", "-o", "interactive", "+o", "zle", "-c", `echo "[$-]"`},
			"[569Xi]\n",
		},
		{
			"and the whole namespace sees it",
			[]string{"zsh", "-o", "interactive", "-c", `[[ -o interactive ]] && [[ -o zle ]] && echo both`},
			"both\n",
		},
		{
			"shinstdin reads back too",
			[]string{"zsh", "--shinstdin", "-c", `[[ -o shinstdin ]] && echo yes`},
			"yes\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, errs bytes.Buffer
			code := driver.MainArgs(zshWriting(&out, &errs), tc.argv)
			if out.String() != tc.want || errs.String() != "" || code != 0 {
				t.Errorf("%v ran %q / said %q status %d, want %q and nothing said",
					tc.argv[1:], out.String(), errs.String(), code, tc.want)
			}
		})
	}
}

// And a running script still may not move any of them, which is the half that
// was always right and is what the rows above are a difference against.
func TestAScriptStillCannotMoveAnyOfThem(t *testing.T) {
	for _, name := range []string{"interactive", "monitor", "shinstdin", "singlecommand", "zle"} {
		t.Run(name, func(t *testing.T) {
			var out, errs bytes.Buffer
			code := driver.MainArgs(zshWriting(&out, &errs), []string{"zsh", "-c", "setopt " + name})
			if want := "can't change option: " + name + "\n"; errs.String() == "" ||
				errs.String()[len(errs.String())-len(want):] != want || code != 1 {
				t.Errorf("setopt %s said %q status %d, want %q at 1", name, errs.String(), code, want)
			}
		})
	}
}
