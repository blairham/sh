// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"
)

// TestAFrozenPrefixIsRefusedWithTheCommand: the redirections and the words are
// done before the frozen name is checked, so a redirection that will not open
// is the only complaint and the line goes on. Measured 2026-10-03 in the
// pinned image. See interp.Semantics.PrefixToAFrozenNameIsCheckedFirst.
func TestAFrozenPrefixIsRefusedWithTheCommand(t *testing.T) {
	out, _ := run(t, "readonly x=1\n( x=2 /bin/echo RAN >/nope/f ); echo \"st=$?\"\n( x=2 /bin/true $(echo ARG >&2) ); echo \"st=$?\"\n")
	if !strings.Contains(out, "can't create /nope/f") || strings.Count(out, "is read only") != 1 ||
		!strings.Contains(out, "st=1\n") || !strings.Contains(out, "ARG\n") || strings.Contains(out, "RAN") {
		t.Errorf("got %q, want the file complaint alone at 1, then ARG before the one refusal", out)
	}
	if strings.Index(out, "ARG") > strings.Index(out, "is read only") {
		t.Errorf("got %q, want the word expanded before the refusal", out)
	}
}
