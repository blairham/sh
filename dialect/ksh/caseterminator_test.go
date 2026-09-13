// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/syntax"
)

// The word straight after a `case`'s `in` is the first arm's pattern here, so
// this shell matches a subject spelled `esac`. Measured 2026-09-12 on ksh93u+
// under `env -i PATH=/usr/bin:/bin` with a scratch HOME, over `-c` and a
// script file alike; the other five refuse the `)`.
//
//	$ ksh s.sh          # case esac in esac) echo hit;; esac
//	hit
func TestTheCaseTerminatorIsAPatternAfterTheHeaderHere(t *testing.T) {
	if !ksh.Dialect().CaseTerminatorIsAPatternAfterTheHeader {
		t.Error("ksh93 reads the word after `in` as a pattern")
	}
	out, st, err := preset.Combined(t, dialecttest.Base{}, "case esac in esac) echo hit;; esac")
	if err != nil {
		t.Fatalf("case esac in esac): %v", err)
	}
	if out != "hit\n" || st != 0 {
		t.Errorf("out = %q (status %d), want %q at 0", out, st, "hit\n")
	}
}

// Which is why a `case` with no arms of its own is refused on one line and
// runs with a newline in front of the `esac`. The pair is the whole rule, and
// the second row is what says it is not "a case must have an arm".
//
//	$ ksh -n s.sh       # case x in esac
//	s.sh: syntax error at line 2: `newline' unexpected
//	$ ksh -n s.sh       # case x in ⏎ esac
//	(nothing)
func TestAnArmlessCaseNeedsANewlineHere(t *testing.T) {
	d := ksh.Dialect()
	for _, tc := range []struct{ src, want string }{
		{"case x in esac\n", "syntax error at line 2: `newline' unexpected"},
		{"case x in esac; echo done\n", "syntax error at line 1: `;' unexpected"},
		// A line continuation is not a newline, so it does not restore the
		// terminator.
		{"case x in \\\nesac\n", "syntax error at line 3: `newline' unexpected"},
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
	for _, src := range []string{
		"case x in\nesac\n",
		"case x in\nesac; echo done\n",
		// A comment ends the line, so the newline after it counts.
		"case x in # c\nesac\n",
		// And an arm after a `;;` closes with an ordinary terminator, the
		// reading being the first arm's alone.
		"case x in y) ;; esac\n",
		// A parenthesized pattern needs no newline: the paren takes the
		// reservation away in every shell.
		"case esac in (esac) echo hit;; esac\n",
	} {
		if _, err := syntax.Parse(src, d); err != nil {
			t.Errorf("%q: %v", src, err)
		}
	}
}

// An unterminated `case` names the arm's terminator, and #2233 filed the two
// rows that pin it as "the newline may be what decides". It is not: what
// decides is whether a command separator stood between the arm's last command
// and the terminator. Measured 2026-09-12 against ksh93u+ 2012-08-01,
// `env -i PATH=/usr/bin:/bin` with a scratch HOME, `-n` over a script file
// ending where each row shows.
//
// Twelve rows rather than the two the issue carried, because the pair
// disagreed over one newline and a rule written from them would have been the
// newline's. See syntax.Parser.terminatorStood.
func TestAnUnterminatedCaseNamesTheArmsTerminatorHere(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		// Nothing between the command and the terminator: the arm closed
		// cleanly, so the `case` is still what is open.
		{"case x in x) : ;;\n", "syntax error at line 2: `case' unmatched"},
		{"case x in x) : ;&\n", "syntax error at line 2: `case' unmatched"},
		// A separator between them, and the terminator is named. All three
		// spellings of one, which is what says it is not the newline.
		{"case x in x) : ; ;;\n", "syntax error at line 2: `;;' unmatched"},
		{"case x in x) : & ;;\n", "syntax error at line 2: `;;' unmatched"},
		{"case x in x) :\n;;\n", "syntax error at line 3: `;;' unmatched"},
		{"case x in x) : ;\n;;\n", "syntax error at line 3: `;;' unmatched"},
		{"case x in x) : ;\n;&\n", "syntax error at line 3: `;&' unmatched"},
		{"case x in x) false || ;\n;;\n", "syntax error at line 3: `;;' unmatched"},
		// It is the arm's own, so a second arm clears the first one's — the
		// pair being what says it does not simply linger.
		{"case x in x) : ;; y) : ; ;;\n", "syntax error at line 2: `;;' unmatched"},
		{"case x in x) : ; ;; y) : ;\n", "syntax error at line 2: `case' unmatched"},
		// And a construct closing over it takes it away, exactly as it takes
		// away a stepped-over separator.
		{"case x in x) : ; ;; esac\n{\n", "syntax error at line 3: `{' unmatched"},
		// An arm with no command in it at all is the terminator's either
		// way, and the one shape that is not is the step-over: a `;` alone
		// in front of the terminator is named, and a newline after that `;`
		// gives the terminator back. That last pair is #2233's row 3 and its
		// neighbor, and they differ by the newline and nothing else.
		{"case x in x) ;;\n", "syntax error at line 2: `;;' unmatched"},
		{"case x in x) ; ;;\n", "syntax error at line 2: `;' unmatched"},
		{"case x in x) ;\n;;\n", "syntax error at line 3: `;;' unmatched"},
		{"case x in x) ; ;&\n", "syntax error at line 2: `;' unmatched"},
		// The arm with no terminator at all is unchanged, which is the
		// control: these are the rows #1207 closed on and they still answer
		// the keyword.
		{"case x in x) false\n", "syntax error at line 2: `case' unmatched"},
		{"case x in x) ;\n", "syntax error at line 2: `case' unmatched"},
		{"case x in x) : ;; y) : ;\n", "syntax error at line 2: `case' unmatched"},
	} {
		_, err := syntax.Parse(c.src, ksh.Dialect())
		if err == nil {
			t.Errorf("%q parsed; this shell refuses it", c.src)
			continue
		}
		if got := ksh.Diagnostics().ParseFailure(err); got != c.want {
			t.Errorf("%q:\n got %q\nwant %q", c.src, got, c.want)
		}
	}
}

// The terminator is what the arm was left open by, so a `case` nested in one
// carries its own — and this shell has no `;;&`, which is the row that says
// the set of terminators here is `;;` and `;&` and nothing else.
func TestANestedCaseCarriesItsOwnArmTerminatorHere(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"case a in a) case b in b) : ; ;;\n", "syntax error at line 2: `;;' unmatched"},
		{"case x in x) : ;\n;;&\n", "syntax error at line 2: `&' unexpected"},
		{"case x in x) ; ;;&\n", "syntax error at line 1: `&' unexpected"},
	} {
		_, err := syntax.Parse(c.src, ksh.Dialect())
		if err == nil {
			t.Errorf("%q parsed; this shell refuses it", c.src)
			continue
		}
		if got := ksh.Diagnostics().ParseFailure(err); got != c.want {
			t.Errorf("%q:\n got %q\nwant %q", c.src, got, c.want)
		}
	}
}

// A bare `!` is an arm that ran no command, and this shell answers it as it
// answers the empty arm rather than as it answers `: ;;`. Measured beside the
// rows above; it is also the one statement with no End() to ask, so a rule
// written over the tree rather than over the source crashed on it.
func TestABareNegationArmIsTheTerminatorsHere(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"case x in x) ! ;;\n", "syntax error at line 2: `;;' unmatched"},
		{"case x in x) ! ; ;;\n", "syntax error at line 2: `;;' unmatched"},
		// And the discriminator beside it: a real command in the same place
		// names the `case`, quoted `;` and all.
		{"case x in x) echo ';' ;;\n", "syntax error at line 2: `case' unmatched"},
		{"case x in x) : # c\n;;\n", "syntax error at line 3: `;;' unmatched"},
	} {
		_, err := syntax.Parse(c.src, ksh.Dialect())
		if err == nil {
			t.Errorf("%q parsed; this shell refuses it", c.src)
			continue
		}
		if got := ksh.Diagnostics().ParseFailure(err); got != c.want {
			t.Errorf("%q:\n got %q\nwant %q", c.src, got, c.want)
		}
	}
}
