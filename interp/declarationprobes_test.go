// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"bytes"
	"context"
	"testing"

	"github.com/blairham/sh/syntax"
)

// TestADeclarationLooksItsNameUpOnce: a declaration in a function asks about
// its name a dozen times — the value, the arrays, the attributes it displaces,
// the store, and all of it again on the return — and each was a hash of the
// name and a probe of the store's index. The store remembers the last name it
// found (Runner.cell), so the run of questions about one name costs one probe.
//
// Three declarations and their return cost 47 probes a call over the same body
// with plain assignments before that, and 8 after it (#6373). Counted, so the
// row is deterministic; the bound has room above what a call costs now and is
// well below what it cost before.
func TestADeclarationLooksItsNameUpOnce(t *testing.T) {
	var out, errOut bytes.Buffer
	r := newTestRunner(t, &Runner{Stdout: &out, Stderr: &errOut})
	run := func(src string) {
		t.Helper()
		f, err := syntax.Parse(src, syntax.Core())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatal(err)
		}
	}
	run(`plain() { c=$1$2; }
declaring() { local a=$1 b=$2 c=; c=$a$b; }`)
	probes := func(call string) int {
		run(call) // the first call makes the cells; count a later one
		before := r.cellProbes
		run(call)
		return r.cellProbes - before
	}
	base := probes("plain x y")
	run("c=g")
	extra := probes("declaring x y") - base
	run(`echo "$c"`)
	if out.String() != "g\n" || errOut.Len() != 0 {
		t.Fatalf("the calls wrote %q and %q, want g and nothing: the local leaked or was refused", out.String(), errOut.String())
	}
	if extra > 12 {
		t.Errorf("three declarations probe the store's index %d times a call more than plain assignments; want at most 12", extra)
	}
}
