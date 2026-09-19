// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/syntax"
)

// A `case` subject that is not a word is named here the way every other
// refused token is, and what would have stood there is a **class** — so it is
// printed bare where a spelling is quoted.
//
// Measured 2026-09-19 on dash 0.5.12, script file under `env -i
// PATH=/usr/bin:/bin LC_ALL=C` with standard input on the null device:
//
//	case ; in x) ;; esac     ";" unexpected (expecting word)
//	case && in x) ;; esac    "&&" unexpected (expecting word)
//	case > in x) ;; esac     redirection unexpected (expecting word)
//	case                     end of file unexpected (expecting word)
//	for in x; do :; done     word unexpected (expecting "do")
//
// The last row is the control that says the quoting is the expectation's and
// not the sentence's: one word is quoted and a class is not, in the same
// clause of the same wording. Before this, the first four were a sentence of
// this parser's own — `expected a word after \`case\“ — in every column
// (#3758).
func TestACaseSubjectRefusalNamesTheTokenAndExpectsABareWord(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"case ; in x) ;; esac\n", `Syntax error: ";" unexpected (expecting word)`},
		{"case && in x) ;; esac\n", `Syntax error: "&&" unexpected (expecting word)`},
		{"case > in x) ;; esac\n", "Syntax error: redirection unexpected (expecting word)"},
		{"case", "Syntax error: end of file unexpected (expecting word)"},
		{"for in x; do :; done\n", `Syntax error: word unexpected (expecting "do")`},
	} {
		_, err := syntax.Parse(tc.src, dash.Dialect())
		if err == nil {
			t.Errorf("%q parsed, want a refusal", tc.src)
			continue
		}
		got := dash.Diagnostics().ParseDiagnostic("<shell>", "", err, tc.src)
		if !strings.Contains(got, tc.want) {
			t.Errorf("%q said %q, want %q", tc.src, got, tc.want)
		}
	}
}
