// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// Where the remark about a here-document the input ended is located, when a
// backslash-newline is what the input ended after: a continued line ends on
// the line the input ran out on, and an operator line continued into the end
// is the last line given up (#6273).
//
// Measured 2026-10-06 against bash 5.3.20, one script file per row: the
// first number is the warning's `line N:` and the second its `at line M`.
func TestAHeredocRemarkCountsTheLinesAContinuationRanInto(t *testing.T) {
	for _, c := range []struct {
		src      string
		line, at int32
	}{
		{"cat <<EOF\nhi \\\n", 3, 1},
		{"cat <<EOF\nhi \\\n\\\n", 4, 1},
		{"cat <<EOF\na\\\nb\n", 3, 1},
		{"cat <<EOF\na\\\nb", 3, 1},
		{"cat <<EOF\na\nb \\\n", 4, 1},
		{"cat <<-EOF\n\thi \\\n", 3, 1},
		{"cat <<EOF; echo x \\\n", 2, 2},
		// The controls: what was already right, and stays so.
		{"cat <<EOF\nhi\n", 2, 1},
		{"cat <<EOF\nhi \\", 2, 1},
		{"cat <<EOF\n\\\n", 2, 1},
		{"cat <<EOF\na\n\\\n", 3, 1},
		{"cat <<'EOF'\nhi \\\n", 2, 1},
	} {
		t.Run(c.src, func(t *testing.T) {
			p := syntax.NewParser(c.src, syntax.Core())
			p.Parse()
			var got *syntax.Remark
			for i, r := range p.Remarks() {
				if r.Kind == syntax.RemarkHeredocAtEOF {
					got = &p.Remarks()[i]
					break
				}
			}
			if got == nil {
				t.Fatal("no remark")
			}
			if got.Pos.Line != c.line || got.At.Line != c.at {
				t.Errorf("line %d at line %d, want line %d at line %d", got.Pos.Line, got.At.Line, c.line, c.at)
			}
		})
	}
}
