// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/syntax"
)

// A process substitution the input ran out inside used to be worded by the
// lexer — one sentence for all four dialects, none of them what its shell
// says, and no line at all because the failure was not a *syntax.Error for a
// line to be read from (#1023).
//
// This shell says about `<(` exactly what it says about `$(`, which is what
// the empty Diagnostics field means: the parentheses hold a program either
// way, and stating that as a fallback rather than as a copied string is what
// keeps the two from drifting apart. So the `$(` rows are here beside them —
// a test of the fallback that does not assert what it falls back to would
// pass with the fallback removed and the string duplicated.
//
// Whole rendered lines, location included. The line is the half the lexer's
// sentence could not carry, and this shell puts an unmatched construct that
// holds a program on the line *after* the input's last.
func TestAnUnmatchedProcessSubstitutionIsWordedLikeACommandSubstitution(t *testing.T) {
	for _, tc := range []struct {
		src, want string
		line      int
	}{
		{"cat <(echo hi\necho after\n", "unexpected EOF while looking for matching `)'", 3},
		{"cat >(echo hi\necho after\n", "unexpected EOF while looking for matching `)'", 3},
		{"v=$(echo hi\necho after\n", "unexpected EOF while looking for matching `)'", 3},
		{"cat <(cat <<EOF\na\n", "unexpected EOF while looking for matching `)'", 3},
	} {
		_, err := syntax.Parse(tc.src, bash.Dialect())
		if err == nil {
			t.Errorf("%q parsed, want a refusal", tc.src)
			continue
		}
		d := bash.Diagnostics()
		if got := d.ParseFailure(err); got != tc.want {
			t.Errorf("%q:\n got %q\nwant %q", tc.src, got, tc.want)
		}
		if got := d.ParseFailureLine(err); got != tc.line {
			t.Errorf("%q: blamed line %d, want %d", tc.src, got, tc.line)
		}
	}
}
