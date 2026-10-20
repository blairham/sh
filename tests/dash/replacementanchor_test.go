// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"testing"

	"github.com/blairham/sh/interp"

	"github.com/blairham/sh/dialect/dash"
)

// dash has no span replacement at all, so neither axis behind it is dash's to
// answer. Measured 2026-09-16 from a script file: `v=abcabc; printf '%s'
// "${v/b/X}"` is `Bad substitution` at 2 with nothing written (#3272).
func TestTheReplacementAnchorIsUnanswerable(t *testing.T) {
	s := dash.Semantics()
	if got := s.ReplacementAnchors; got != interp.Unspecified {
		t.Errorf("ReplacementAnchors = %v, want unspecified", got)
	}
	if got := s.AnchoredEmptyReplacementPattern; got != interp.Unspecified {
		t.Errorf("AnchoredEmptyReplacementPattern = %v, want unspecified", got)
	}
	// And neither is the global spelling that would carry a second anchor
	// (#3307).
	if got := s.GlobalReplacementAnchors; got != interp.Unspecified {
		t.Errorf("GlobalReplacementAnchors = %v, want unspecified", got)
	}
	if dash.Dialect().ParamSubstitution {
		t.Error("ParamSubstitution is set, but dash calls ${v/b/X} a bad substitution")
	}
}
