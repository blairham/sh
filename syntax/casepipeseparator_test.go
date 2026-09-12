// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"errors"
	"testing"

	"github.com/blairham/sh/syntax"
)

// Inside a `case` arm's parentheses one dialect reads the `|` as the
// alternation separator and stops there, so the character after it begins a
// token of its own rather than joining the pipe into `|&`.
//
// Measured 2026-09-12 on zsh 5.9.2 with `-n` over a script file, against bash
// 5.3.15 and ksh93u+ which lex the pair:
//
//	case a in (a|&b) …    zsh `&`     bash / ksh93 `|&`
//	case a in (a|&|b) …   zsh `&|`    bash / ksh93 `|&`
//	case a in (a|&&b) …   zsh `&&`    — the discriminator
//	case a in a|&|b) …    zsh `|&`    — the control
//
// See Dialect.CasePatternListPipeIsOnlyASeparator (#1111).

// pipeSeparator is the core with the pair readable and the rule on, which is
// the only combination where the rule can be seen at all.
func pipeSeparator(only bool) syntax.Dialect {
	d := syntax.Core()
	d.PipeBothStreams = true
	// `&|` too, so the second row below can be the pair the dialect that has
	// this rule really reads there rather than a bare `&`.
	d.BackgroundAndDisown = true
	d.CasePatternListPipeIsOnlyASeparator = only
	return d
}

func blamedToken(t *testing.T, src string, d syntax.Dialect) string {
	t.Helper()
	_, err := syntax.Parse(src, d)
	if err == nil {
		t.Fatalf("parse %q: no error, want a refusal", src)
	}
	var se *syntax.Error
	if !errors.As(err, &se) {
		t.Fatalf("parse %q: err = %v, want a *syntax.Error", src, err)
	}
	return se.Token
}

func TestAPipeInAParenthesizedPatternListDoesNotJoinWhatFollows(t *testing.T) {
	for _, tc := range []struct{ src, with, without string }{
		{"case a in (a|&b) echo m;; esac", "&", "|&"},
		{"case a in (a|&|b) echo m;; esac", "&|", "|&"},
		// The discriminator: the rule belongs to the `|` that stopped
		// reading and not to the `&` that follows it, so an `&&` after the
		// separator is still one token.
		{"case a in (a|&&b) echo m;; esac", "&&", "|&"},
		// And the control: written without the arm's parentheses the same
		// characters lex as the pair in that dialect too.
		{"case a in a|&|b) echo m;; esac", "|&", "|&"},
	} {
		if got := blamedToken(t, tc.src, pipeSeparator(true)); got != tc.with {
			t.Errorf("%q: blamed %q, want %q", tc.src, got, tc.with)
		}
		if got := blamedToken(t, tc.src, pipeSeparator(false)); got != tc.without {
			t.Errorf("%q without the flag: blamed %q, want %q", tc.src, got, tc.without)
		}
	}
}

// It is a lexical rule and nothing that parses is read differently by it:
// `|&` cannot stand in a pattern list under either reading, so every line the
// flag changes was already refused.
func TestTheSeparatorRuleChangesNoLineThatParses(t *testing.T) {
	for _, src := range []string{
		"case a in (a|b) echo m;; esac",
		"case a in (a) echo m;; esac",
		"echo one |& cat",
		"case a in (a) echo m |& cat;; esac",
	} {
		for _, only := range []bool{false, true} {
			if _, err := syntax.Parse(src, pipeSeparator(only)); err != nil {
				t.Errorf("parse %q with the flag %v: %v", src, only, err)
			}
		}
	}
}
