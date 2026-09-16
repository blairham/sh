// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// `>;` is one dialect's operator and nobody else's, so the flag has to change
// what the text *means* rather than only what is accepted: with it off the
// two bytes fall back to `>` and `;`, which is a redirection with no target
// and the syntax error bash 5.3.15, zsh 5.9.2 and dash all report.
//
// Measured 2026-09-14. That fallback is the hazard the AmpersandRedirect
// comment names — one spelling, two readings — so both are pinned here rather
// than only the one being added (#918).
func TestTheRenameOnSuccessOperatorIsBehindItsFlag(t *testing.T) {
	t.Parallel()
	const src = "echo x >; f"
	t.Run("off", func(t *testing.T) {
		p := syntax.NewParser(src, syntax.Core())
		p.Parse()
		if p.Err() == nil {
			t.Error("accepted `>;` without the flag; it is `>` and `;` there")
		}
	})
	t.Run("on", func(t *testing.T) {
		d := syntax.Core()
		d.RenameOnSuccessRedirect = true
		p := syntax.NewParser(src, d)
		f := p.Parse()
		if err := p.Err(); err != nil {
			t.Fatalf("parse: %v", err)
		}
		sc := f.Stmts[0].Expr.(*syntax.Pipeline).Cmds[0].(*syntax.SimpleCmd)
		if len(sc.Redirs) != 1 {
			t.Fatalf("redirections = %d, want 1", len(sc.Redirs))
		}
		if got := sc.Redirs[0].Op; got != syntax.TokGreatSemi {
			t.Errorf("operator = %v, want >;", got)
		}
		if got := sc.Redirs[0].Word.Literal(); got != "f" {
			t.Errorf("target = %q, want f", got)
		}
	})
}

// The `;` is part of the operator and has to be tight against the `>`, and
// there is no `>>;` and no `<;`. All three are syntax errors in ksh93 too, so
// the flag adds one operator rather than a marker that generalizes — the
// mistake `ClobberOverrideMarker` records making once already.
func TestTheOperatorIsOneSpellingAndNotAMarker(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	d.RenameOnSuccessRedirect = true
	for _, src := range []string{"echo x > ; f", "echo x >>; f", "echo x <; f"} {
		t.Run(src, func(t *testing.T) {
			p := syntax.NewParser(src, d)
			p.Parse()
			if p.Err() == nil {
				t.Errorf("accepted %q; ksh93 refuses it", src)
			}
		})
	}
}

// And it is written back as it was read. A printer that lost the `;` would
// turn a write that only lands on success into one that always lands, which
// is the failure a round trip through the parser cannot see.
func TestTheOperatorIsPrintedBack(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	d.RenameOnSuccessRedirect = true
	f, err := syntax.Parse("echo a >; b", d)
	if err != nil {
		t.Fatal(err)
	}
	if got := syntax.Print(f); got != "echo a >; b" {
		t.Errorf("printed %q, want %q", got, "echo a >; b")
	}
}
