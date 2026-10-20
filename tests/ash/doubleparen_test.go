// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/syntax"
)

// A `$((` whose count closes on a `)` with no second one behind it is refused
// while reading, where `$(( (1+2) ))` is arithmetic. Measured 2026-10-04 in
// the pinned image: `echo "[$((echo ab cde) )]"` and `echo "[$(( (1+2)) )]"`
// are `syntax error: missing '))'` before anything runs (#5723).
func TestADoubleParenWantsItsDoubleCloser(t *testing.T) {
	for _, src := range []string{`echo "[$((echo ab cde) )]"`, `echo "[$(( (1+2)) )]"`, `echo $((1+2) )`} {
		if _, err := syntax.Parse(src, ash.Dialect()); err == nil {
			t.Errorf("%s parsed, want `missing '))'`", src)
		}
	}
	if _, err := syntax.Parse(`echo "[$(( (1+2) ))]"`, ash.Dialect()); err != nil {
		t.Errorf("$(( (1+2) )) refused: %v", err)
	}
}
