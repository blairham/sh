// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// `functions -M` is exclusive, and the refusal must not depend on whether the
// other letter is one this engine has **built**.
//
// **That is the noun the case in `dialect/zsh` was keyed on for a year**
// (#5073). It read `functions -Mt` and grepped for `not implemented yet`, so
// what it graded was the missing-letter table and not exclusivity: the moment
// `-t` was implemented the assertion would have started failing for a correct
// shell, and until then `functions -Mu mf 1 1 g` registered `mf` in silence at
// 0 with nothing to notice.
//
// So the rows here hold the letter fixed and move it **between the two
// tables**. A letter the dialect spells is refused by the *set* either way; a
// letter it spells in neither is refused by name. Nothing about the answer may
// move with the table.
func TestTheMathLetterIsExclusiveWhicheverTableTheOtherLetterIsIn(t *testing.T) {
	run := func(t *testing.T, accepted, unimplemented string) (string, int) {
		t.Helper()
		var buf bytes.Buffer
		sem := CoreSemantics()
		sem.FunctionsOptions = accepted
		sem.DeclareMappingLetter = DeclareMappingLetterRegistersAMathFunction
		// Answered so the run says nothing about it: the builtin resolves a
		// name, and an unanswered axis on that road would put its own
		// complaint into the output these rows read.
		sem.EmptyPathIsTheCurrentDirectory = No
		dg := Diagnostics{
			MarkingUnderPlusRefusal:    "invalid option(s)",
			UnimplementedOptionLetters: map[string]string{"functions": unimplemented},
		}
		r := newTestRunner(t, &Runner{
			Stdout: &buf, Stderr: &buf,
			Semantics: &sem, Diagnostics: &dg,
			Dir: t.TempDir(), Name: "testsh",
		})
		// `functions` is a dialect's word, so the core runner has to be given
		// it before any of this is reachable.
		r.Register("functions", FunctionsBuiltin())
		f, err := syntax.Parse("g(){ :; }\nfunctions -Mq mf 1 1 g\n", syntax.Core())
		if err != nil {
			t.Fatal(err)
		}
		st, err := r.Run(context.Background(), f)
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		return buf.String(), st
	}
	t.Run("the letter is one this engine accepts", func(t *testing.T) {
		out, st := run(t, "mMsq", "")
		if !strings.Contains(out, "invalid option(s)") {
			t.Errorf("out = %q, want the set refused", out)
		}
		if st != 1 {
			t.Errorf("status = %d, want 1", st)
		}
	})
	t.Run("and the same letter waiting to be built", func(t *testing.T) {
		// The identical answer, which is the whole point: moving `q` from one
		// table to the other must not change it.
		out, st := run(t, "mMs", "q")
		if !strings.Contains(out, "invalid option(s)") {
			t.Errorf("out = %q, want the set refused", out)
		}
		if st != 1 {
			t.Errorf("status = %d, want 1", st)
		}
	})
	t.Run("while a letter in neither table is refused by name", func(t *testing.T) {
		// The row that says the two refusals are different sentences. Without
		// it, a rule that refused every non-companion letter would pass both
		// rows above and be wrong about every letter the shell has not got.
		out, _ := run(t, "mMs", "")
		if strings.Contains(out, "invalid option(s)") {
			t.Errorf("out = %q, want the letter refused by name rather than the set", out)
		}
		if !strings.Contains(out, "-q") {
			t.Errorf("out = %q, want the letter named", out)
		}
	})
	t.Run("and a dialect with no refusal wording says nothing new", func(t *testing.T) {
		// The zero value: a dialect that does not word this refusal keeps the
		// behavior it had, which is what stops the rule reaching a column
		// that never asked for it.
		var buf bytes.Buffer
		sem := CoreSemantics()
		sem.FunctionsOptions = "mMsq"
		sem.DeclareMappingLetter = DeclareMappingLetterRegistersAMathFunction
		sem.EmptyPathIsTheCurrentDirectory = No
		r := newTestRunner(t, &Runner{
			Stdout: &buf, Stderr: &buf,
			Semantics: &sem, Diagnostics: &Diagnostics{},
			Dir: t.TempDir(), Name: "testsh",
		})
		// `functions` is a dialect's word, so the core runner has to be given
		// it before any of this is reachable.
		r.Register("functions", FunctionsBuiltin())
		f, err := syntax.Parse("g(){ :; }\nfunctions -Mq mf 1 1 g\n", syntax.Core())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatalf("run: %v", err)
		}
		if strings.Contains(buf.String(), "invalid option(s)") {
			t.Errorf("out = %q, want no refusal from a dialect that words none", buf.String())
		}
	})
}
