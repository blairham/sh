// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

func reach(r BareNegationReach) Dialect {
	d := Core()
	d.BareNegationReach = r
	return d
}

// A `!` with no pipeline after it is a pipeline with no commands in it. Three
// reaches take it and each takes a different set of positions, so every row
// below is a row for all three.
func TestWhereABareNegationMayStand(t *testing.T) {
	// The columns are the three accepting reaches; the core takes none.
	for _, tc := range []struct {
		src                       string
		terminator, listEnd, both bool
	}{
		{"!\n", true, true, true},
		{"! ; echo x\n", true, true, true},
		{"!", true, true, true}, // the end of input
		{"! & echo x\n", true, false, true},
		{"{ ! ; echo x; }\n", true, true, true},
		{"( ! )\n", false, true, true},
		{"{ ! }\n", false, true, true},
		{"case x in x) ! ;; esac\n", false, true, true},
		{"! && echo two\n", false, true, true},
		{"! || echo two\n", false, true, true},
		// A bar is refused by every column, which is the discriminating
		// half: a reach written for "any operator" would take four lines
		// no shell does.
		{"! | cat\n", false, false, false},
	} {
		for _, col := range []struct {
			name string
			d    Dialect
			want bool
		}{
			{"terminator", reach(BareNegationBeforeATerminator), tc.terminator},
			{"list end", reach(BareNegationWhereAListEnds), tc.listEnd},
			{"either", reach(BareNegationAtEitherPlace), tc.both},
			{"none", Core(), false},
		} {
			_, err := Parse(tc.src, col.d)
			if got := err == nil; got != col.want {
				t.Errorf("%s: %q parsed = %v, want %v (%v)", col.name, tc.src, got, col.want, err)
			}
		}
	}
}

// The `!` is never swallowed. Where the reach does not admit it the refusal
// names the token that came after it, which is what every column of the panel
// that refuses does — and returning quietly instead left an empty program
// that exited 0.
func TestABareNegationTheReachRefusesIsNamed(t *testing.T) {
	for _, tc := range []struct {
		src  string
		kind ErrorKind
		col  int
	}{
		// The end of input is the one that is not a token in the way: the
		// `!` is still open when the text stops, which is the unterminated
		// kind and is what dash's `end of file unexpected` renders from.
		{"!", ErrUnterminated, 2},
		{"!\n", ErrUnexpected, 2},
		{"! ; echo x\n", ErrUnexpected, 3},
		{"! | cat\n", ErrUnexpected, 3},
	} {
		_, err := Parse(tc.src, Core())
		if err == nil {
			t.Errorf("%q parsed under the core, want a refusal", tc.src)
			continue
		}
		e, ok := err.(*Error)
		if !ok {
			t.Errorf("%q: err = %T, want *Error", tc.src, err)
			continue
		}
		if e.Kind != tc.kind {
			t.Errorf("%q: kind = %v, want %v", tc.src, e.Kind, tc.kind)
		}
		// Where it stopped is where the token after the `!` stands, not
		// where the `!` does: the refusal names what it found.
		if int(e.Pos.Col) != tc.col {
			t.Errorf("%q: refused at column %d, want %d", tc.src, e.Pos.Col, tc.col)
		}
	}
}

// A second `!` toggles the first where the dialect says so, which is why one
// flag on the tree is enough: an even count is no negation at all.
func TestRepeatedNegationToggles(t *testing.T) {
	d := reach(BareNegationBeforeATerminator)
	d.RepeatedNegationToggles = true
	for _, tc := range []struct {
		src  string
		want bool
	}{
		{"! true\n", true},
		{"! ! true\n", false},
		{"! ! ! true\n", true},
		{"! !\n", false},
		{"! ! !\n", true},
	} {
		f, err := Parse(tc.src, d)
		if err != nil {
			t.Errorf("%q: %v", tc.src, err)
			continue
		}
		pl, ok := f.Stmts[0].Expr.(*Pipeline)
		if !ok {
			t.Errorf("%q: %T, want *Pipeline", tc.src, f.Stmts[0].Expr)
			continue
		}
		if pl.Negated != tc.want {
			t.Errorf("%q: negated = %v, want %v", tc.src, pl.Negated, tc.want)
		}
	}
	// Without the flag a second `!` is refused, and it is named as the
	// reserved word it is rather than as an ordinary one.
	off := reach(BareNegationBeforeATerminator)
	_, err := Parse("! ! true\n", off)
	if err == nil {
		t.Fatal("`! ! true` parsed without the toggle")
	}
	e, ok := err.(*Error)
	if !ok {
		t.Fatalf("err = %T, want *Error", err)
	}
	if e.Class != ClassReserved {
		t.Errorf("class = %v, want ClassReserved — `!` is a reserved word", e.Class)
	}
}
