// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/internal/oracle"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The corpus, printed back and then *run*, against what the original does.
//
// Re-parsing is the weaker half of the promise and it is the half that looks
// convincing. A dropped here-document body re-parses perfectly and reads from
// the terminal instead of from the body; a lost `&` re-parses and waits. Only
// running both says whether the printed source means what it said.
//
// Under one dialect rather than the core, because the core refuses whatever
// the shells disagree about and most of the corpus is about exactly that —
// the question here is the printer, not the semantics.
func TestPrintedSourceStillMeansTheSameThing(t *testing.T) {
	for _, c := range oracle.Corpus {
		if c.SyntaxError {
			continue
		}
		if c.LayoutSensitive {
			// What this case pins is where a diagnostic points, which is a
			// fact about the source's layout. The printer promises meaning,
			// not layout, and its zero layout is nobody's — so running the
			// printed form and comparing the line it named would be checking
			// the wrong promise.
			continue
		}
		t.Run(c.ID, func(t *testing.T) {
			f, err := syntax.Parse(c.Snippet, bash.Dialect())
			if err != nil {
				t.Skipf("not this dialect: %v", err)
			}
			printed := syntax.Print(f)

			// A directory each, because a case that creates a file would
			// find it already there on the second run — and the name taken
			// out of what they printed, because two directories differ by
			// their names and that is not the printer's doing either.
			wantOut, wantStatus := runUnderBash(t, c.Snippet)
			gotOut, gotStatus := runUnderBash(t, printed)
			if gotOut != wantOut || gotStatus != wantStatus {
				t.Errorf("printing changed what it does\n  from:   %s\n  gave:   %s\n  before: %q (%d)\n  after:  %q (%d)",
					c.Snippet, printed, wantOut, wantStatus, gotOut, gotStatus)
			}
		})
	}
}

// runUnderBash runs a snippet the way the conformance harness would, and
// returns what it printed and reported.
func runUnderBash(t *testing.T, src string) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, bash.Dialect())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var out bytes.Buffer
	dir := t.TempDir()
	sem, dg := bash.Semantics(), bash.Diagnostics()
	r := &interp.Runner{
		Semantics: &sem, Diagnostics: &dg,
		Stdout: &out, Stderr: &out,
		Stdin: strings.NewReader(""),
		// TMPDIR is the Runner's own, so a corpus case with a process
		// substitution in it makes its directory somewhere the framework
		// takes away — not in a real one, where nothing calls CleanUp.
		// Its own directory rather than Dir: the path a substitution
		// expands to would otherwise be rewritten by the <dir> masking
		// below, which is meant for what the *snippet* prints.
		Name: "sh", Dir: dir, Env: append(testPATH(), "TMPDIR="+t.TempDir()),
	}
	bash.Apply(r)
	status, rerr := r.Run(context.Background(), f)
	text := strings.ReplaceAll(out.String(), dir, "<dir>")
	if rerr != nil {
		return text + "refused: " + rerr.Error(), -1
	}
	return text, status
}
