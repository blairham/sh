// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/syntax"
)

// A `case` subject that is not a word is named here the way every other
// refused token is, what would have stood there is a **class** and so is
// printed bare, and a refused newline is blamed on the line it ends.
//
// All three were missing from this column at once, which is why they are one
// test: the first was a sentence of this parser's own, the second is the tail
// TestTheKeywordWithNoNameNamesTheTokenHere deferred to the `case` refusal,
// and the third is a Diagnostics value this dialect never held while dash
// carried it.
//
// Measured 2026-09-19 against BusyBox ash 1.37.0 in
// alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b,
// script files with standard input on the null device:
//
//	case ; in x) ;; esac     line 1: unexpected ";" (expecting word)
//	case && in x) ;; esac    line 1: unexpected "&&" (expecting word)
//	case > in x) ;; esac     line 1: unexpected redirection (expecting word)
//	case                     line 1: unexpected end of file (expecting word)
//	case\nin x) ;; esac      line 2: unexpected newline (expecting word)
//	case x in x) :;; zzz\n   line 2: unexpected newline (expecting ")")
//	!\n                      line 2: unexpected newline
//	for in x; do :; done     line 1: unexpected word (expecting "do")
//
// The last row is the control that says the quoting is the expectation's and
// not the sentence's: one word is quoted and a class is not, in the same
// clause of the same wording. The two `zzz` and `!` rows are the control that
// says the line is every newline's and not this production's (#3758).
func TestACaseSubjectRefusalNamesTheTokenAndExpectsABareWord(t *testing.T) {
	for _, tc := range []struct {
		src, want string
		line      int
	}{
		{"case ; in x) ;; esac\n", `syntax error: unexpected ";" (expecting word)`, 1},
		{"case && in x) ;; esac\n", `syntax error: unexpected "&&" (expecting word)`, 1},
		{"case > in x) ;; esac\n", "syntax error: unexpected redirection (expecting word)", 1},
		{"case", "syntax error: unexpected end of file (expecting word)", 1},
		{"case\nin x) ;; esac\n", "syntax error: unexpected newline (expecting word)", 2},
		{"case x in x) :;; zzz\n", `syntax error: unexpected newline (expecting ")")`, 2},
		{"!\n", "syntax error: unexpected newline", 2},
		{"for in x; do :; done\n", `syntax error: unexpected word (expecting "do")`, 1},
	} {
		_, err := syntax.Parse(tc.src, ash.Dialect())
		if err == nil {
			t.Errorf("%q parsed, want a refusal", tc.src)
			continue
		}
		d := ash.Diagnostics()
		if got := d.ParseDiagnostic("f.sh", "", err, tc.src); !strings.Contains(got, tc.want) {
			t.Errorf("%q said %q, want %q", tc.src, got, tc.want)
		}
		if got := d.ParseFailureLine(err); got != tc.line {
			t.Errorf("%q is on line %d, want %d", tc.src, got, tc.line)
		}
	}
}
