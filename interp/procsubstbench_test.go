// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp_test

import (
	"context"
	"io"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The cost of a process substitution whose body runs nothing external, which
// is the path the goroutine bodies exist to make fast: flat, nested, and two
// siblings each numbering one of their own at once (#6543).
func BenchmarkProcessSubstitution(b *testing.B) {
	for _, c := range []struct{ name, src string }{
		{"flat", `: <(true) <(true)`},
		{"nested", `: <(: <(true)) <(: <(true))`},
		{"siblings", `: <(echo <(true)) <(echo <(true))`},
	} {
		b.Run(c.name, func(b *testing.B) {
			d := syntax.Core()
			d.ProcessSubstitution = true
			f, err := syntax.Parse(c.src, d)
			if err != nil {
				b.Fatal(err)
			}
			sem := PosixSemantics()
			sem.SubstitutionEndPlacement = SubstitutionEndsWhereAnyDescriptorGoes
			sem.FirstAllocatedDescriptor = AllocateDescriptorsFromTen
			sem.OwnDescriptors = SavedStreamsAndPipeReadEndsTakeNumbers
			// testrunner:bare — a benchmark has no *testing.T for the
			// helper, and nothing here touches the directory or TMPDIR.
			r := &Runner{Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Stdout: io.Discard, Stderr: io.Discard}
			b.ReportAllocs()
			for b.Loop() {
				if _, err := r.Run(context.Background(), f); err != nil {
					b.Fatal(err)
				}
			}
			r.CleanUp()
		})
	}
}
