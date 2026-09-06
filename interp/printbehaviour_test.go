// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"

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
// Under a wider grammar than the core, because the core refuses whatever the
// shells disagree about and most of the corpus is about exactly that — the
// question here is the printer, not the semantics.
func TestPrintedSourceStillMeansTheSameThing(t *testing.T) {
	unread := 0
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
			f, err := syntax.Parse(c.Snippet, corpusGrammar())
			if err != nil {
				unread++
				t.Skipf("the grammar here does not read it: %v", err)
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
	// A skip is invisible in a passing run, so a grammar that quietly stopped
	// reading half the corpus would read as a green round-trip over half as
	// many cases. Every case here parses today; if one stops, that is the
	// finding rather than a smaller test.
	if unread != 0 {
		t.Errorf("%d corpus cases did not parse; corpusGrammar no longer reads the corpus", unread)
	}
}

// corpusGrammar is the grammar the round-trip reads the corpus with: the core,
// widened by every construct the corpus actually contains.
//
// Named here rather than taken from a dialect. What this test needs is a
// grammar wide enough to reach the cases — which shell happens to have that
// grammar is not the printer's business, and borrowing one made `interp`'s
// tests stop compiling without `dialect/bash` (#491). The list is the corpus's
// requirement and the check above is what keeps it honest: today it reads all
// 1270 of the cases that are neither syntax errors nor layout-sensitive.
func corpusGrammar() syntax.Dialect {
	d := syntax.Core()
	d.CaseContinue = true
	d.Coproc = true
	d.CoprocName = true
	d.CurrentShellSubstitution = true
	d.DollarDoubleQuote = true
	d.ExtendedPatternInCondition = true
	d.FdVariableSubscript = true
	d.FuncDefAtParen = true
	d.FunctionKeywordParens = true
	d.MultiDigitFdNumber = true
	d.ParamCaseChange = true
	d.ParamIndirection = true
	d.ParamTransformations = true
	d.RegexTakesAlternation = true
	d.TimePosixFlag = true
	// `declare` beside the four the core already reads as declarations, so a
	// snippet using it keeps the assignment rule when it is printed back.
	d.DeclarationUtilities = map[string]bool{
		"declare": true, "export": true, "local": true,
		"readonly": true, "typeset": true,
	}
	return d
}

// guardedBuffer is a buffer that may be read while the shell is still writing
// it.
//
// `Run` returning does not mean every goroutine the script started has
// finished: a background job outlives the script that started it, which is
// what a background job is for, and in a real shell it is a separate process
// that goes on writing to the terminal after the shell has gone. Here it is a
// goroutine holding the caller's io.Writer, so reading that writer the
// instant Run returns is a read racing a write.
//
// The shell's own lock cannot help — it guards the writers it hands out, and
// this is the far side of one. So the buffer carries its own, and the same
// lock covers the String that reads it.
type guardedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (g *guardedBuffer) Write(p []byte) (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.buf.Write(p)
}

func (g *guardedBuffer) String() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.buf.String()
}

// runUnderBash runs a snippet the way the conformance harness would, and
// returns what it printed and reported.
func runUnderBash(t *testing.T, src string) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, corpusGrammar())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var out guardedBuffer
	dir := t.TempDir()
	sem, dg := testSemantics(), interp.PosixDiagnostics()
	r := newTestRunner(t, &interp.Runner{
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
	})
	status, rerr := r.Run(context.Background(), f)
	text := strings.ReplaceAll(out.String(), dir, "<dir>")
	if rerr != nil {
		return text + "refused: " + rerr.Error(), -1
	}
	return text, status
}
