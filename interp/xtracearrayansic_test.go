// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// Diagnostics.TraceArrayLiteralDecodesAnsiCQuoting: a `$'…'` element of a
// bare array literal is written as the characters it stands for.
//
// The bare literal is the one place a trace shows the words the script wrote
// rather than the values they came to, so it is the one place the question can
// be asked at all. Measured 2026-09-22 on bash 5.3.20 from a script file with
// `set -x` on line 1 and the trace read byte for byte.
//
// Named for the field and never for a shell.
func TestAnArrayLiteralCanDecodeItsAnsiCQuoting(t *testing.T) {
	for _, tc := range []struct {
		name    string
		src     string
		off, on string
	}{
		{
			"a tab",
			"set -x\na=( $'\\t' )\n",
			"+ a=($'\\t')\n",
			"+ a=('\t')\n",
		},
		{
			"the span and not the word",
			"set -x\na=( x$'\\t'y )\n",
			"+ a=(x$'\\t'y)\n",
			"+ a=(x'\t'y)\n",
		},
		{
			// A value holding a quote takes the ordinary single-quoted
			// spelling of one, which is what makes this a re-quoting rather
			// than a substitution of the text.
			"a quote inside the value",
			"set -x\na=( $'a\\'b' )\n",
			"+ a=($'a\\'b')\n",
			"+ a=('a'\\''b')\n",
		},
		{
			// Every other spelling is the script's own, on either answer:
			// the rule reaches the one span kind and nothing else.
			"the other spellings are untouched",
			"set -x\na=( \"d\\$q\" '' \"*\" 'a'b )\n",
			"+ a=(\"d\\$q\" '' \"*\" 'a'b)\n",
			"+ a=(\"d\\$q\" '' \"*\" 'a'b)\n",
		},
		{
			// And a scalar keeps the spelling whatever the field says: its
			// trace is the value, quoted by TraceQuoting.
			"a scalar is not this question",
			"set -x\nb=$'\\t'\n",
			"+ b=$'\\t'\n",
			"+ b=$'\\t'\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			off := traceOf(t, tc.src, permissive(), Diagnostics{TraceQuoting: QuoteDollar})
			if off != tc.off {
				t.Errorf("with the field off: %q, want %q", off, tc.off)
			}
			on := traceOf(t, tc.src, permissive(), Diagnostics{
				TraceQuoting:                         QuoteDollar,
				TraceArrayLiteralDecodesAnsiCQuoting: true,
			})
			if on != tc.on {
				t.Errorf("with the field on: %q, want %q", on, tc.on)
			}
		})
	}
}
