// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"io"
	"testing"
)

// A stream over several targets is still that while a builtin holds its
// output, which is how `exec` sees it: `exec >a >b; exec /bin/echo hi` was
// replaced with standard output closed, because the hold hid the marker.
func TestAMultiTargetStreamIsSeenThroughAHold(t *testing.T) {
	multi := multiTarget{Writer: io.Discard}
	if !isMultiTarget(&heldOutput{to: multi, out: multi}) {
		t.Error("a held multi-target stream read as one descriptor")
	}
	if isMultiTarget(&heldOutput{to: io.Discard, out: io.Discard}) {
		t.Error("a held ordinary stream read as multi-target")
	}
}
