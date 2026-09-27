// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"context"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

// Whether a `case` arm's parenthesized pattern list is read as one word is
// decided while the line is *read*, so it is a dialect field rather than a
// semantics axis — and one dialect spells it as an option a running script
// switches. That makes the front end the only place the two can meet, exactly
// as it is for the doubled-quote reading beside this file.
//
// The observable is one line of output: `wide` means the list was read as one
// word and the blank inside it matched, and a refusal means it was read the
// standard's way.

const casePatternListWord = "case 'a b' in (a b) printf 'wide\\n';; *) printf 'narrow\\n';; esac\n"

// casePatternListShell has a builtin that moves the reading, because no core
// `set -o` name reaches it: the field exists for a dialect's own option
// namespace and the question here is the front end's.
func casePatternListShell() driver.Shell {
	sh := shell()
	sh.Register = func(r *interp.Runner) {
		r.Register("widelist", func(r *interp.Runner, _ context.Context, args []string) int {
			r.SetCasePatternListReadAsOneWord(len(args) == 0 || args[0] != "off")
			return 0
		})
	}
	return sh
}

func TestABuiltinCanDecideHowTheNextLineReadsACasePatternList(t *testing.T) {
	// The core has no such reading, so the word is refused until something
	// asks for it — which is also the control that says the two rows below
	// are not both the same answer.
	t.Run("nothing asked for it", func(t *testing.T) {
		_, _, code := runArgs(t, casePatternListShell(), "testsh",
			writeScript(t, casePatternListWord))
		if code == 0 {
			t.Errorf("the list parsed with the core's reading, so the rows below cannot show a change")
		}
	})

	// A script file, which is the route the reader takes a line at a time:
	// the line after the builtin is read the new way.
	t.Run("a script file reads the next line the new way", func(t *testing.T) {
		out, errs, code := runArgs(t, casePatternListShell(), "testsh",
			writeScript(t, "widelist\n"+casePatternListWord))
		if code != 0 || out != "wide\n" {
			t.Errorf("got %q status %d, stderr %q, want %q", out, code, errs, "wide\n")
		}
	})

	// And back again, which is what makes it a switch rather than a build.
	t.Run("and back again", func(t *testing.T) {
		_, _, code := runArgs(t, casePatternListShell(), "testsh",
			writeScript(t, "widelist\nwidelist off\n"+casePatternListWord))
		if code == 0 {
			t.Errorf("the list still parsed after the reading was put back, so it only ever widens")
		}
	})

	// The control that says the *reading* moved and not the word: a line
	// already read is read the old way, however the line ends.
	t.Run("a line already read keeps its reading", func(t *testing.T) {
		_, _, code := runArgs(t, casePatternListShell(), "testsh",
			writeScript(t, "widelist; "+casePatternListWord))
		if code == 0 {
			t.Errorf("a list on the builtin's own line was read the new way, so the moment is not the read")
		}
	})
}
