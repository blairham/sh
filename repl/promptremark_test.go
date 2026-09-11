// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// What the parser had to say about input it accepted anyway reaches a prompt.
//
// The one remark the panel has belongs to the end of the input: a
// here-document whose delimiter never arrived is accepted and run by every
// shell, and bash warns about it — measured 2026-09-11 under `-i`,
// `bash: warning: here-document at line 1 delimited by end-of-file (wanted
// `EOT')`, written before the body runs. The script routes have said it since
// there were remarks and the prompt had no path to say it at all, so the body
// ran and nothing was said (#1892).
func TestARemarkReachesThePrompt(t *testing.T) {
	var ran, said strings.Builder
	r := newTestRunner(map[string]string{"PS1": "", "PS2": "", "PATH": "/bin:/usr/bin"})
	r.Stdout = &ran
	var seen []syntax.Remark
	s := Shell{
		Runner: r,
		In:     strings.NewReader("cat <<EOT\nbody\n"),
		Out:    &ran,
		Err:    &said,
		Remark: func(rk syntax.Remark) string {
			seen = append(seen, rk)
			return "testsh: warning\n"
		},
	}
	if _, err := s.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 1 || seen[0].Kind != syntax.RemarkHeredocAtEOF {
		t.Fatalf("remarks = %+v, want the one about a here-document", seen)
	}
	// The remark names where the here-document *began*, which is not where
	// the input ran out: `At` is line 1 and `Pos` is line 2, and a dialect's
	// wording reads the first while the location reads the second.
	if seen[0].At.Line != 1 || seen[0].Pos.Line != 2 {
		t.Errorf("remark at %v, began at %v, want line 2 and line 1", seen[0].Pos, seen[0].At)
	}
	if got, want := said.String(), "testsh: warning\n"; got != want {
		t.Errorf("said %q, want %q", got, want)
	}
	// And the command still runs, which is the half the warning is about:
	// every shell in the panel accepts the construct.
	if got, want := ran.String(), "body\n"; got != want {
		t.Errorf("output %q, want %q", got, want)
	}
}

// A caller that was told nothing says nothing, which is three of the four
// dialects and every embedder without one.
func TestNoRemarkSeamSaysNothing(t *testing.T) {
	out, errs, st := endOfInputSession(t, "cat <<EOT\nbody\n")
	if errs != "" {
		t.Errorf("said %q with no seam to say it through", errs)
	}
	if out != "body\n" || st != 0 {
		t.Errorf("output %q status %d, want %q at 0", out, st, "body\n")
	}
}
