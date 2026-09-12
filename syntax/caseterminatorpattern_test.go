// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

func withEsacAsAPattern() Dialect {
	d := Core()
	d.CaseTerminatorIsAPatternAfterTheHeader = true
	return d
}

// Where the flag is on, the word straight after a `case`'s `in` is the first
// arm's pattern however it is spelled, so a `case` may match the text `esac`.
// That is the acceptance the flag is for; the refusal below is its shadow.
func TestTheCaseTerminatorIsAPatternDirectlyAfterIn(t *testing.T) {
	const src = "case esac in esac) echo hit;; esac\n"
	if _, err := Parse(src, withEsacAsAPattern()); err != nil {
		t.Errorf("%q: %v", src, err)
	}
	if _, err := Parse(src, Core()); err == nil {
		t.Errorf("%q parsed under the core, want a refusal", src)
	}
}

// The shadow: with the word read as a pattern there is no terminator left, so
// a `case` written with none of its own on one line is refused.
func TestAnArmlessCaseOnOneLineIsRefusedWithTheFlag(t *testing.T) {
	for _, src := range []string{
		"case x in esac\n",
		"case x in esac; echo done\n",
		// A line continuation is not a newline, so the reading survives it.
		"case x in \\\nesac\n",
	} {
		if _, err := Parse(src, withEsacAsAPattern()); err == nil {
			t.Errorf("%q parsed; the `esac` is a pattern here", src)
		}
		if _, err := Parse(src, Core()); err != nil {
			t.Errorf("%q: refused without the flag: %v", src, err)
		}
	}
}

// A newline gives the word its reservation back, and that is what keeps this
// from being a rule about a `case` needing an arm: with one written in front
// of the `esac` the same armless `case` parses.
func TestANewlineRestoresTheCaseTerminator(t *testing.T) {
	for _, src := range []string{
		"case x in\nesac\n",
		"case x in\nesac; echo done\n",
		// A comment ends the line, so the newline after it counts.
		"case x in # c\nesac\n",
	} {
		for name, d := range map[string]Dialect{
			"flag": withEsacAsAPattern(), "core": Core(),
		} {
			if _, err := Parse(src, d); err != nil {
				t.Errorf("%s: %q: %v", name, src, err)
			}
		}
	}
}

// Only the first arm's position, and only before any arm has been read: an
// `esac` after a `;;` is the terminator again.
func TestOnlyTheFirstArmTakesTheTerminatorAsAPattern(t *testing.T) {
	for _, src := range []string{
		"case x in y) ;; esac) echo hit;; esac\n",
		"case x in\nesac) echo hit;; esac\n",
	} {
		if _, err := Parse(src, withEsacAsAPattern()); err == nil {
			t.Errorf("%q parsed; the `esac` is the terminator here", src)
		}
	}
	// And an arm whose pattern is parenthesized needs no flag for it: the
	// paren takes the reservation away in every dialect.
	for name, d := range map[string]Dialect{
		"flag": withEsacAsAPattern(), "core": Core(),
	} {
		const src = "case esac in (esac) echo hit;; esac\n"
		if _, err := Parse(src, d); err != nil {
			t.Errorf("%s: %q: %v", name, src, err)
		}
	}
}
