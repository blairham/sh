// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// bareFlags is the core plus the unbraced flag sigils. The flag is named here
// and the shell that sets it is not.
func bareFlags() syntax.Dialect {
	d := syntax.Core()
	d.BareParamFlags = true
	return d
}

// bareFlagsAndSubscript is both, for the rows about a subscript on a flagged
// parameter. They are separate questions and a grammar could answer them
// differently, so the combination is written out rather than folded into one
// flag.
func bareFlagsAndSubscript() syntax.Dialect {
	d := bareFlags()
	d.BareSubscript = true
	return d
}

// The whole of the flag: `$=v` is one expansion where it is on and a literal
// `$` followed by `=v` where it is off.
//
// Checked on the spans rather than on a result, because the word boundary is
// the entire difference — once the spans are cut, nothing downstream can tell
// the two readings apart.
func TestAnUnbracedFlagSigilIsPartOfTheExpansion(t *testing.T) {
	for _, tc := range []struct {
		name  string
		src   string
		value string // the expansion's inner text, as `${ … }` would hold it
		after string // the literal text left over, "" for none
	}{
		{"the existence flag", "echo $+v", "+v", ""},
		{"the split flag", "echo $=v", "=v", ""},
		{"the glob flag", "echo $~v", "~v", ""},
		{"the distribute flag", "echo $^v", "^v", ""},
		{"text after it", "echo $+vx", "+vx", ""},
		{"text the name cannot hold", "echo $+v-x", "+v", "-x"},
		{
			// `+` takes a name or a digit and nothing else, so this is not
			// an expansion at all — measured, zsh 5.9.2 prints `$+@`.
			name: "the existence flag on a special is text", src: "echo $+@",
			value: "", after: "$+@",
		},
		{"the existence flag doubled is text", "echo $++v", "", "$++v"},
		{"the existence flag alone is text", "echo $+", "", "$+"},
		{
			// The other three take every target a bare `$` does.
			name: "the split flag on a special", src: "echo $=@",
			value: "=@", after: "",
		},
		{"the glob flag on a digit", "echo $~1", "~1", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spans := spansOf(t, tc.src, bareFlags())
			var value, after string
			for _, s := range spans {
				switch s.Kind {
				case syntax.ParamExp:
					value = s.Value
				default:
					after += s.Value
				}
			}
			if value != tc.value || after != tc.after {
				t.Errorf("%q: expansion %q with %q after, want %q and %q",
					tc.src, value, after, tc.value, tc.after)
			}
		})
	}
}

// Off, every one of them is text — which is what the flag is for, and what
// makes it a grammar question rather than a value on the semantics vector.
func TestWithoutTheFlagAnUnbracedSigilIsText(t *testing.T) {
	for _, src := range []string{"echo $+v", "echo $=v", "echo $~v", "echo $^v"} {
		spans := spansOf(t, src, syntax.Core())
		for _, s := range spans {
			if s.Kind == syntax.ParamExp {
				t.Errorf("%q: parsed an expansion %q without the flag", src, s.Value)
			}
		}
	}
}

// A subscript still belongs to the parameter under the sigil, which needs the
// sigil trimmed before the name is looked at — without it the brackets fell
// out of the expansion and `$+a[1]` came apart into `$+a` and `[1]`.
func TestAFlaggedParameterStillTakesASubscript(t *testing.T) {
	for _, tc := range []struct{ src, value string }{
		{"echo $+a[1]", "+a[1]"},
		{"echo $=a[1]", "=a[1]"},
		{"echo $~a[1]", "~a[1]"},
		{"echo $^a[1]", "^a[1]"},
		{"echo $#a[1]", "#a[1]"},
		{"echo $a[1]", "a[1]"},
	} {
		spans := spansOf(t, tc.src, bareFlagsAndSubscript())
		if len(spans) != 1 || spans[0].Kind != syntax.ParamExp || spans[0].Value != tc.value {
			t.Errorf("%q: spans %+v, want one expansion %q", tc.src, spans, tc.value)
		}
	}
}
