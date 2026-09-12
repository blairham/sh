// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"errors"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/syntax"
)

// TestTheGrammarSaysWhenAShellHasNeitherPipeAmpersandNorCaseFallthrough grades
// the bash preset with its two youngest operators turned off against the
// oldest bash anyone has to run on.
//
// bash 3.2.57 — what macOS ships, and the panel's `bash32` column — has
// neither `|&` nor `;&`. It is one absence seen twice: a shell without the
// operator lexes the two characters apart, so where the pair stands somewhere
// the grammar refuses it, 3.2 names the *first character* and 5.3 names the
// pair. #2406 filed that as a thing the grammar could not express. It can:
// `PipeBothStreams` and `CaseFallthrough` are the two flags, the fallback to
// `|` then `&` is already what the operator table does, and this is the test
// that says so rather than the doc comment claiming it.
//
// Measured 2026-09-12, `env -i PATH=/usr/bin:/bin` with a scratch HOME, `-n`
// over a script file, bash 5.3.15 against bash 3.2.57:
//
//	probe                                     bash 5.3   bash 3.2
//	a |& b                                    runs       `&'
//	case x in (a) echo 1;& (b) echo 2;; esac  runs       `&'
//	|& echo hi                                `|&'       `|'
//	;& echo hi                                `;&'       `;'
//	if |& :; then :; fi                       `|&'       `|'
//	if ;& :; then :; fi                       `;&'       `;'
//
// Rows one and two are the cause and the other four are the consequence. The
// two positions matter and both are here: after a command the `|` is fine and
// the `&` behind it is not, and at the front of a list the `|` is already
// wrong, which is why the naming moves.
//
// What this does *not* do is add a bash32 preset or a sixth binary. Whether
// one should exist is a decision about what this shell ships, and it is
// separate from whether the grammar can say it — which is what #2406 asked
// and what this answers.
func TestTheGrammarSaysWhenAShellHasNeitherPipeAmpersandNorCaseFallthrough(t *testing.T) {
	old := bash.Dialect()
	old.PipeBothStreams = false
	old.CaseFallthrough = false
	for _, c := range []struct {
		name, src, old, cur string
	}{
		{
			"a pipe carrying both streams",
			"a |& b\n", "&", "",
		},
		{
			"a case arm falling through",
			"case x in (a) echo 1;& (b) echo 2;; esac\n", "&", "",
		},
		{
			// The pair where no command stands in front of it, which is
			// where the naming shows.
			"a pipe where a command belongs",
			"|& echo hi\n", "|", "|&",
		},
		{
			"a fallthrough where a command belongs",
			";& echo hi\n", ";", ";&",
		},
		{
			// And inside a keyword's condition, so it is not a property of
			// the front of a file.
			"a pipe where an if wants its condition",
			"if |& :; then :; fi\n", "|", "|&",
		},
		{
			"a fallthrough where an if wants its condition",
			"if ;& :; then :; fi\n", ";", ";&",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := refusedToken(t, c.src, old); got != c.old {
				t.Errorf("with the operators off, %q named %q, want %q",
					c.src, got, c.old)
			}
			if got := refusedToken(t, c.src, bash.Dialect()); got != c.cur {
				t.Errorf("with them on, %q named %q, want %q",
					c.src, got, c.cur)
			}
		})
	}
}

// refusedToken is the token a parse refusal named, or "" where the source
// parsed. Reported rather than asserted, so that a row which stops being a
// refusal at all is a wrong *token* and not a skipped assertion.
func refusedToken(t *testing.T, src string, d syntax.Dialect) string {
	t.Helper()
	_, err := syntax.Parse(src, d)
	if err == nil {
		return ""
	}
	var se *syntax.Error
	if !errors.As(err, &se) {
		t.Fatalf("parsing %q: %v, want a *syntax.Error", src, err)
	}
	return se.Token
}
