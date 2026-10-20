// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/syntax"
)

// This shell writes a `case` header with braces too, and unlike zsh it
// **pairs** the two words: a `{` closes with a `}` and an `in` with an `esac`,
// and neither mixture parses. Measured 2026-09-12 on ksh93u+ under `env -i
// PATH=/usr/bin:/bin` with a scratch HOME.
//
//	$ ksh -c 'case x { x) echo hit;; }'
//	hit
//	$ ksh -c 'case x { x) echo hit;; esac'
//	ksh: syntax error at line 1: `case' unmatched
func TestABraceSpelledCasePairsHere(t *testing.T) {
	d := ksh.Dialect()
	if got := d.CaseBraceBody; got != syntax.CaseBraceBodyPairsWithItsOpener {
		t.Errorf("spelling = %v, want CaseBraceBodyPairsWithItsOpener", got)
	}
	for _, tc := range []struct{ src, want string }{
		{"case x { x) echo hit;; }", "hit"},
		{"case y { x) echo hit;; *) echo star;; }", "star"},
		{"case x { }; echo st=$?", "st=0"},
		{"case x {\nx) echo hit;;\n}", "hit"},
		{"case x { (x) echo hit;; }", "hit"},
		{"case x { x) echo one;; } ; echo done", "one\ndone"},
		// The word after the header is a pattern here whichever opener was
		// written, so a subject spelled `esac` matches after a `{` as it
		// does after an `in`. zsh refuses this line.
		{"case esac { esac) echo hit;; }", "hit"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s: said %q, want %q", tc.src, got, tc.want)
		}
	}
	// Written without a trailing newline, which is the `-c` shape these were
	// measured in: over a *file* the shell names the newline it met instead,
	// and both spellings agree with it there too.
	for _, tc := range []struct{ src, want string }{
		// Neither mixture, which is the whole of the split with zsh.
		{"case x { x) echo hit;; esac", "syntax error at line 1: `case' unmatched"},
		{"case x in x) echo hit;; }", "syntax error at line 1: `case' unmatched"},
		// The `{` needs a blank after it, this shell not having the rule
		// that makes a bare `{` the reserved word however the text runs on.
		{"case x {x) echo hit;; }", "syntax error at line 1: `{x' unexpected"},
		// And an arm's last command needs its terminator, `}` being an
		// ordinary word where a command's arguments stand.
		{"case x { x) echo hit }", "syntax error at line 1: `case' unmatched"},
		// An `esac` after the `{` is a pattern, so there is no closer left.
		{"case x { esac", "syntax error at line 1: `case' unmatched"},
	} {
		_, err := syntax.Parse(tc.src, d)
		if err == nil {
			t.Errorf("%q parsed; this shell refuses it", tc.src)
			continue
		}
		if got := ksh.Diagnostics().ParseFailure(err); got != tc.want {
			t.Errorf("%q:\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
