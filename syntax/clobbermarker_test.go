// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// markerDialect is a core dialect with the clobber-override marker turned on,
// and AmpersandRedirect with it because four of the seven spellings are the
// marker on `&>` and `&>>` and cannot be reached without those operators.
func markerDialect() Dialect {
	d := Core()
	d.ClobberOverrideMarker = true
	d.AmpersandRedirect = true
	return d
}

// TestClobberOverrideMarkerReadsEverySpelling covers all seven the flag adds,
// each as the operator of a single redirection rather than merely "it
// parsed": the bug this was written for parsed cleanly and meant something
// else (#1247).
func TestClobberOverrideMarkerReadsEverySpelling(t *testing.T) {
	for _, tc := range []struct {
		src string
		op  Kind
	}{
		{"echo hi >! f", TokClobberBang},
		{"echo hi >>| f", TokDGreatClobber},
		{"echo hi >>! f", TokDGreatBang},
		{"echo hi &>| f", TokAmpGreatClobber},
		{"echo hi &>! f", TokAmpGreatBang},
		{"echo hi &>>| f", TokAmpDGreatClobber},
		{"echo hi &>>! f", TokAmpDGreatBang},
		// `>|` is core rather than the flag's, and is here as the control:
		// the spelling that already worked must still read the same way.
		{"echo hi >| f", TokClobber},
	} {
		rs, args := redirsOf(t, tc.src, markerDialect())
		if len(rs) != 1 || rs[0].Op != tc.op {
			t.Errorf("%q: got %v, want one redirection with op %v", tc.src, opsOf(rs), tc.op)
			continue
		}
		if target := litOf(rs[0].Word); target != "f" {
			t.Errorf("%q: redirection target %q, want f", tc.src, target)
		}
		if len(args) != 2 {
			t.Errorf("%q: command words %v, want just the name and hi — the target leaked into the arguments",
				tc.src, args)
		}
	}
}

// TestWithoutTheMarkerABangSpellingIsAWordAndNotAnError is the whole reason
// the flag exists. The marker's two spellings fall back differently, and this
// is the silent one: with the flag off, `>! f` is a redirection to a file
// *called* `!` with `f` left as an argument, which parses, runs, reports
// success and writes the wrong file.
//
// Asserting the AST rather than "it parsed" is the point. A test that only
// checked for a syntax error here would have passed against the bug.
func TestWithoutTheMarkerABangSpellingIsAWordAndNotAnError(t *testing.T) {
	for _, src := range []string{"echo hi >! f", "echo hi >>! f", "echo hi &>! f"} {
		rs, args := redirsOf(t, src, Core())
		if len(rs) != 1 {
			t.Errorf("%q: got %v, want exactly one redirection", src, opsOf(rs))
			continue
		}
		if target := litOf(rs[0].Word); target != "!" {
			t.Errorf("%q: redirection target %q, want the bang as a filename", src, target)
		}
		if len(args) != 3 || args[2] != "f" {
			t.Errorf("%q: command words %v, want the target left as a third word", src, args)
		}
	}
}

// TestWithoutTheMarkerAPipeSpellingIsRefused is the other fallback, and it is
// not silent: a `|` marker falls back to a pipe with nothing on its left.
func TestWithoutTheMarkerAPipeSpellingIsRefused(t *testing.T) {
	for _, src := range []string{"echo hi >>| f", "echo hi &>| f", "echo hi &>>| f"} {
		d := Core()
		d.AmpersandRedirect = true
		mustFail(t, src, d, "without the marker a trailing | opens a pipe")
	}
}

// TestTheMarkerOnBothStreamsNeedsBothStreams pins the composition. A marker
// cannot attach to an operator the dialect does not read, so with `&>` off
// these four spellings are not the flag's business — `&>|` is `&` and then
// `>|`, which is a background command and a redirection, exactly as it is in
// the columns that have no `&>`.
func TestTheMarkerOnBothStreamsNeedsBothStreams(t *testing.T) {
	d := Core()
	d.ClobberOverrideMarker = true
	d.AmpersandRedirect = false
	f, err := Parse("echo hi &>| f", d)
	if err != nil {
		t.Fatalf("&>| without &>: %v", err)
	}
	if len(f.Stmts) != 2 {
		t.Fatalf("&>| without &>: %d statements, want 2 — a background command and a redirection",
			len(f.Stmts))
	}
	if !f.Stmts[0].Background {
		t.Error("&>| without &>: the first statement is not backgrounded, so the & was not read as one")
	}
	rs := f.Stmts[1].Expr.(*Pipeline).Cmds[0].(*SimpleCmd).Redirs
	if len(rs) != 1 || rs[0].Op != TokClobber {
		t.Errorf("&>| without &>: got %v, want a plain >| on the second statement", opsOf(rs))
	}
	// And the plain spellings are unaffected by the other flag.
	rs, _ = redirsOf(t, "echo hi >>! f", d)
	if len(rs) != 1 || rs[0].Op != TokDGreatBang {
		t.Errorf("got %v, want >>! read with the marker regardless of &>", opsOf(rs))
	}
}

// TestTheMarkerBindsToADescriptorNumber covers the one place a redirection's
// operator is easiest to lose: an explicit descriptor in front of it. The
// number belongs to the operator, and the marker is part of the operator.
func TestTheMarkerBindsToADescriptorNumber(t *testing.T) {
	rs, args := redirsOf(t, "echo hi 2>! f", markerDialect())
	if len(rs) != 1 || rs[0].Op != TokClobberBang {
		t.Fatalf("got %v, want one >! redirection", opsOf(rs))
	}
	if rs[0].N == nil || litOf(rs[0].N) != "2" {
		t.Errorf("descriptor %v, want 2", rs[0].N)
	}
	if len(args) != 2 {
		t.Errorf("command words %v, want the target out of the arguments", args)
	}
}

// TestEveryMarkerSpellingPrintsBackAsWritten keeps the printer honest. The
// spellings are separate kinds precisely so a redirection round-trips as the
// author wrote it; a shared kind would rewrite `>!` as `>|`, which means the
// same thing to one dialect and something else entirely to the rest.
func TestEveryMarkerSpellingPrintsBackAsWritten(t *testing.T) {
	for _, op := range []string{">|", ">!", ">>|", ">>!", "&>|", "&>!", "&>>|", "&>>!"} {
		src := "echo hi " + op + " f"
		f, err := Parse(src, markerDialect())
		if err != nil {
			t.Errorf("%q: %v", src, err)
			continue
		}
		if got := Print(f); got != src {
			t.Errorf("Print(%q) = %q, want it back unchanged", src, got)
		}
	}
}

// TestTheMarkerIsARedirectKind guards the classifier the parser lifts
// redirections out of a word list with. A kind missing from IsRedirect lexes
// and then is not treated as a redirection at all, which is the silent
// failure again one layer down.
func TestTheMarkerIsARedirectKind(t *testing.T) {
	for _, k := range []Kind{
		TokClobberBang, TokDGreatClobber, TokDGreatBang,
		TokAmpGreatClobber, TokAmpGreatBang, TokAmpDGreatClobber, TokAmpDGreatBang,
	} {
		if !k.IsRedirect() {
			t.Errorf("%v is not classified as a redirection", k)
		}
		if k.String() == "unknown token" {
			t.Errorf("%v has no spelling in the operator table", Kind(k))
		}
	}
}

// redirsOf parses one simple command and returns its redirections and the
// text of its words, which is what the assertions above are about: an
// operator that is read wrongly moves a word between the two.
func redirsOf(t *testing.T, src string, d Dialect) ([]*Redirect, []string) {
	t.Helper()
	f, err := Parse(src, d)
	if err != nil {
		t.Fatalf("%q: %v", src, err)
	}
	if len(f.Stmts) != 1 {
		t.Fatalf("%q: %d statements, want 1", src, len(f.Stmts))
	}
	pipe, ok := f.Stmts[0].Expr.(*Pipeline)
	if !ok || len(pipe.Cmds) != 1 {
		t.Fatalf("%q: statement is %T, want one command", src, f.Stmts[0].Expr)
	}
	cmd, ok := pipe.Cmds[0].(*SimpleCmd)
	if !ok {
		t.Fatalf("%q: command is %T, want a simple command", src, pipe.Cmds[0])
	}
	words := make([]string, 0, len(cmd.Args))
	for _, w := range cmd.Args {
		words = append(words, litOf(w))
	}
	return cmd.Redirs, words
}

// litOf is the target or argument as written, which is all these assertions
// need: every word in them is a bare literal.
func litOf(w *Word) string {
	if w == nil {
		return "<none>"
	}
	return w.Literal()
}

func opsOf(rs []*Redirect) []string {
	ops := make([]string, 0, len(rs))
	for _, r := range rs {
		ops = append(ops, r.Op.String()+" "+litOf(r.Word))
	}
	return ops
}
