// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/internal/dialecttest"
)

// The one variable this shell names itself in. A bare `set` under BusyBox ash
// lists `BB_ASH_VERSION='1.37.0'` first, and there is no `$ASH_VERSION` and no
// `${.sh.version}` — so a script testing for this shell tests for this name,
// and a dialect that did not set it would be invisible to that test.
func TestThePreludeNamesTheShell(t *testing.T) {
	out, st, err := preset.CombinedWithPrelude(t, dialecttest.Base{Name: "ash"},
		`echo "[$BB_ASH_VERSION]"`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0 (%q)", st, out)
	}
	if !strings.Contains(out, ash.Version) {
		t.Errorf("BB_ASH_VERSION = %q, want it to carry %q", out, ash.Version)
	}
	// The tag, which says whose shell this is rather than which BusyBox it
	// was measured against. Both halves are load-bearing: the number tells a
	// reader which binary to re-run, and the tag stops a script concluding it
	// is talking to BusyBox itself.
	if !strings.Contains(ash.Version, "blairham") {
		t.Errorf("Version = %q, want the tag in it", ash.Version)
	}
}

// And the prelude defines nothing else. `pushd` and `popd` are what bash and
// zsh add and what this shell, like dash, does not have — a dialect is as much
// what it declines to provide.
func TestThePreludeDefinesNoDirectoryStack(t *testing.T) {
	if strings.Contains(ash.Prelude(), "pushd") {
		t.Errorf("Prelude = %q, want no pushd", ash.Prelude())
	}
}
