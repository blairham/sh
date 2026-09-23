// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// `printf` writes through the stream guard, so a substitution's body writing at
// the same moment cannot race it.
//
// The guard is what a real shell gets from the kernel: two writers on one
// descriptor are serialized by it, and an embedder's io.Writer has no such
// property — which is the whole reason lockedWriter exists. `printf` reached the
// stream directly instead, so on a caller's plain writer the two parties were one
// *locked* writer, on the body's goroutine, and one bare `io.WriteString`, on the
// shell's, onto the same sink. CI reported it as `strings.(*Builder).Write` under
// lockedWriter against `printfWriter.write`, on a snippet whose only crime was a
// `printf` beside a `<( … )`.
//
// It is one guard and not a wait: nothing joins a body — streamseal.go has the
// measurement for why no shell in the panel waits for one — so the answer is that
// both writers take the same lock, not that one of them finishes first.
//
// **The sink here is a bare strings.Builder on purpose.** The helper every other
// test in this file uses holds a mutex of its own, which is why nothing here ever
// saw this: a guarded sink makes the bug unreachable. An embedder's writer is not
// guarded, and that is the shape this asserts. Run under `-race`; without it the
// output is merely plausible.
func TestPrintfTakesTheStreamGuardABodyAlsoTakes(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	d.ProcessSubstitution = true
	// The body's own command cannot be found, so it writes a diagnostic to the
	// shell's standard error from its goroutine while `printf` is writing
	// standard output from the shell's.
	const src = `for i in 1 2 3 4 5 6 7 8; do
	printf '[%s]' "$(echo x <(nosuchcmd-doesnotexist))"
done`
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var buf strings.Builder
	sem := testSemantics()
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Env: testPATH(), Dialect: &d,
	})
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := strings.Count(buf.String(), "[x "); got != 8 {
		t.Errorf("got %q with %d passes, want 8", buf.String(), got)
	}
}
