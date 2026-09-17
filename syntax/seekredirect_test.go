// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// `<#` and `>#` are one dialect's operators and nobody else's, so the flag
// has to change what the text *means* rather than only what is accepted:
// with it off the `#` begins a comment, which leaves the redirection with no
// target at all and is the syntax error the other three report.
//
// Measured 2026-09-16 on ksh93u+ 2012-08-01, bash 5.3.20, zsh 5.9 and dash
// 0.5.12. That fallback is the hazard AmpersandRedirect's comment names — one
// spelling, two readings — so both are pinned here rather than only the one
// being added (#3034).
func seekDialect() syntax.Dialect {
	d := syntax.Core()
	d.SeekRedirect = true
	return d
}

func TestTheSeekOperatorsAreBehindTheirFlag(t *testing.T) {
	t.Parallel()
	for _, src := range []string{"exec 3<#((0))", "exec 4>#((3))"} {
		t.Run(src, func(t *testing.T) {
			p := syntax.NewParser(src, syntax.Core())
			p.Parse()
			if p.Err() == nil {
				t.Error("accepted the operator without the flag; the `#` opens a comment there")
			}
			p = syntax.NewParser(src, seekDialect())
			if f := p.Parse(); p.Err() != nil {
				t.Fatalf("parse: %v", p.Err())
			} else if got := syntax.Print(f); got != src {
				t.Errorf("printed back as %q, want %q", got, src)
			}
		})
	}
}

// The operand is an arithmetic command and the tree says so: the word is the
// one ArithSubst span `$((…))` carries, which is what makes the offset an
// ordinary expansion rather than a second evaluator.
func TestTheSeekOperandIsAnArithmeticCommand(t *testing.T) {
	t.Parallel()
	p := syntax.NewParser("exec 3<# ((n * 3 + 1))", seekDialect())
	f := p.Parse()
	if err := p.Err(); err != nil {
		t.Fatalf("parse: %v", err)
	}
	sc := f.Stmts[0].Expr.(*syntax.Pipeline).Cmds[0].(*syntax.SimpleCmd)
	if len(sc.Redirs) != 1 {
		t.Fatalf("redirections = %d, want 1", len(sc.Redirs))
	}
	rd := sc.Redirs[0]
	if rd.Op != syntax.TokLessHash {
		t.Errorf("operator = %v, want <#", rd.Op)
	}
	if got := rd.N.Literal(); got != "3" {
		t.Errorf("descriptor = %q, want 3", got)
	}
	if len(rd.Word.Spans) != 1 || rd.Word.Spans[0].Kind != syntax.ArithSubst {
		t.Fatalf("operand spans = %+v, want one ArithSubst", rd.Word.Spans)
	}
	if got := rd.Word.Spans[0].Value; got != "n * 3 + 1" {
		t.Errorf("expression = %q, want %q", got, "n * 3 + 1")
	}
	if got := rd.Text; got != "((n * 3 + 1))" {
		t.Errorf("text = %q, want the parentheses it was written with", got)
	}
	// The blank between the operator and the operand is not kept, because
	// nothing about the construct turns on it.
	if got := syntax.Print(f); got != "exec 3<#((n * 3 + 1))" {
		t.Errorf("printed back as %q", got)
	}
}

// ksh93 reads a *pattern* after the operator as well, and this grammar does
// not claim that half: the word is refused rather than read as an offset it
// is not. `exec 3<#0` segfaults ksh93u+ 2012-08-01 outright, so there is no
// behaviour there to match either.
func TestASeekOperandThatIsNotArithmeticIsRefused(t *testing.T) {
	t.Parallel()
	for _, src := range []string{"exec 3<#0", "echo hi >#x", `exec 3<#"0"`, "exec 3<#$((0))"} {
		t.Run(src, func(t *testing.T) {
			p := syntax.NewParser(src, seekDialect())
			p.Parse()
			if p.Err() == nil {
				t.Errorf("accepted %q as a seek offset", src)
			}
		})
	}
}

// One operator each way and no marker that generalizes: there is no `<<#`,
// no `>>#` and no `&>#`, and the `#` has to be tight against the `<`.
func TestTheSeekOperatorsAreTwoSpellingsAndNotAMarker(t *testing.T) {
	t.Parallel()
	d := seekDialect()
	d.AmpersandRedirect = true
	for _, src := range []string{"exec 3< #((0))", "exec 3>>#((0))", "exec 3&>#((0))"} {
		t.Run(src, func(t *testing.T) {
			p := syntax.NewParser(src, d)
			p.Parse()
			if p.Err() == nil {
				t.Errorf("accepted %q", src)
			}
		})
	}
	// `<<` is the longer operator and keeps the text, so `cat <<#` is a
	// here-document with a `#` where its delimiter goes rather than a `<`
	// followed by a seek. Neither reading parses, which is the point: the
	// text has to fail the *same* way with the flag on as without it, or the
	// two-byte match moved.
	const heredoc = "cat <<#\n"
	off := syntax.NewParser(heredoc, syntax.Core())
	off.Parse()
	on := syntax.NewParser(heredoc, d)
	on.Parse()
	if off.Err() == nil || on.Err() == nil {
		t.Fatalf("`cat <<#` parsed: off=%v on=%v", off.Err(), on.Err())
	}
	if off.Err().Error() != on.Err().Error() {
		t.Errorf("`<<#` reads differently with the flag on: %v, want %v", on.Err(), off.Err())
	}
}
