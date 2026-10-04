// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"errors"
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/syntax"
)

// TestAnUnterminatedBraceInAHeredocBodyIsRefusedWithTheLine: the program is
// refused when it is read, at the line the input ends on — the `${` reads on
// past the delimiter looking for its `}`. Measured 2026-10-03 in the pinned
// image. See syntax.Dialect.HeredocBodyBraceIsReadWithTheLine.
func TestAnUnterminatedBraceInAHeredocBodyIsRefusedWithTheLine(t *testing.T) {
	for _, tc := range []struct {
		src  string
		line int32
	}{
		{"cat <<EOF\nW${\nEOF\n", 3},
		{"cat <<EOF\nW${\nEOF\necho after\n", 4},
		{"echo x\ncat <<EOF\nW${\nEOF\n\n\necho after\n", 7},
		{"cat <<EOF\nW${\nEOF\necho after", 3},
	} {
		_, err := syntax.Parse(tc.src, ash.Dialect())
		var se *syntax.Error
		if !errors.As(err, &se) || se.Pos.Line != tc.line {
			t.Errorf("%q: err = %v, want a refusal at line %d", tc.src, err, tc.line)
		}
	}
	if !parses(t, "cat <<'EOF'\nW${\nEOF\n") {
		t.Errorf("a quoted body is text and is not refused")
	}
}
