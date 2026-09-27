// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"errors"
	"testing"
)

// What a here-document's delimiter may be written as — see
// [HeredocDelimiterSubstitution], which carries the panel.
//
// Nothing in a delimiter expands anywhere, so the question is only whether the
// parser accepts the shapes an expansion is written in. Three readings, and
// `<<"$(a b)"` is the row that says there are three rather than two: one
// reading takes a quoted substitution and refuses an unquoted one, the other
// refuses it either way.
func withDelimiterSubstitutions(a HeredocDelimiterSubstitution) Dialect {
	d := Core()
	d.HeredocDelimiterSubstitutions = a
	return d
}

// The whole grid, read three ways. A row is either taken or refused, and the
// two refusing readings disagree about four of the nine.
func TestWhatAHeredocDelimiterMayBeWrittenAsIsADialectQuestion(t *testing.T) {
	t.Parallel()
	const (
		ok      = "taken"
		refused = "refused"
	)
	for _, c := range []struct {
		name, delim              string
		every, unquoted, cmdSubs string
	}{
		{
			name: "an unquoted command substitution", delim: "$(echo E)",
			every: ok, unquoted: refused, cmdSubs: refused,
		},
		{
			// The two characters and not the expression: the reading that
			// scans nothing unquoted leaves the `(` standing whatever
			// follows it, and the one that refuses a *command* substitution
			// takes this because it is not one.
			name: "an unquoted arithmetic expansion", delim: "$((1))",
			every: ok, unquoted: refused, cmdSubs: ok,
		},
		{
			name: "a command substitution inside a word", delim: "a$(echo E)b",
			every: ok, unquoted: refused, cmdSubs: refused,
		},
		{
			// The row that parts the two refusing readings, and the reason
			// one flag could not hold both.
			name: "a quoted command substitution", delim: `"$(echo E)"`,
			every: ok, unquoted: ok, cmdSubs: refused,
		},
		{
			// A backquote with no blank in it: the reading that scans
			// nothing unquoted still takes this, because what it costs is
			// the protection of a blank and there is none here.
			name: "a backquoted substitution of one word", delim: "`a`",
			every: ok, unquoted: ok, cmdSubs: ok,
		},
		{
			name: "a quoted backquoted substitution", delim: "\"`a b`\"",
			every: ok, unquoted: ok, cmdSubs: ok,
		},
		{
			// The control that says this is about the substitution
			// spellings and not about a `$` in a delimiter at all.
			name: "a braced parameter", delim: "${v}",
			every: ok, unquoted: ok, cmdSubs: ok,
		},
		{
			name: "a bare parameter", delim: "$v",
			every: ok, unquoted: ok, cmdSubs: ok,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			src := ": <<" + c.delim + "\nbody\nEND\necho alive\n"
			for _, r := range []struct {
				a    HeredocDelimiterSubstitution
				want string
			}{
				{HeredocDelimiterTakesEverySubstitution, c.every},
				{HeredocDelimiterScansNoUnquotedSubstitution, c.unquoted},
				{HeredocDelimiterRefusesACommandSubstitution, c.cmdSubs},
			} {
				_, err := Parse(src, withDelimiterSubstitutions(r.a))
				if got := ok; err != nil {
					got = refused
					if got != r.want {
						t.Errorf("%v: %q = %v, want it %s", r.a, src, err, r.want)
					}
				} else if got != r.want {
					t.Errorf("%v: %q was %s, want it %s", r.a, src, got, r.want)
				}
			}
		})
	}
}

// A backquote in the unquoted part of a delimiter is an ordinary character
// under one reading, and the consequence is not a refusal of the backquote: it
// is that the delimiter **ends at the blank**, which leaves the backquote open
// and swallows the rest of the input.
//
// The row that says so is the pair — “ `a` “ is taken and “ `a b` “ is
// not, and the difference between them is a space. A rule written as "refuse a
// backquote in a delimiter" passes the second and fails the first.
func TestABackquoteInAnUnquotedDelimiterDoesNotProtectABlank(t *testing.T) {
	t.Parallel()
	d := withDelimiterSubstitutions(HeredocDelimiterScansNoUnquotedSubstitution)
	if _, err := Parse(": <<`a`\nbody\nEND\n", d); err != nil {
		t.Errorf("a backquoted word with no blank in it = %v, want it read", err)
	}
	var se *Error
	_, err := Parse(": <<`a b`\nbody\nEND\n", d)
	if err == nil || !errors.As(err, &se) {
		t.Fatalf("a backquoted word with a blank in it = %v, want a refusal", err)
	}
	// Input that ran out with a construct still open, rather than anything
	// about the delimiter: the word ended at the blank and left the
	// backquote behind it open, and the rest of the input went into it. The
	// position is where the construct was opened; the line a front end
	// *reports* for one that ran out is the end of the input, which is where
	// the reference names it too.
	if se.Kind != ErrUnmatched {
		t.Errorf("kind = %v, want ErrUnmatched", se.Kind)
	}
}

// And what the refusing reading says, which is the sentence and the kind one
// dialect already words for a here-document a substitution could not feed.
//
// The token is `<<` and the delimiter with its quoting off, which every other
// here-document refusal in this grammar follows — and which is an
// approximation of the reference's own, for the reason
// [HeredocDelimiterRefusesACommandSubstitution] records.
func TestARefusedDelimiterSubstitutionIsTheHeredocSentence(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ src, token string }{
		{": <<$(echo E)\nbody\nEND\n", "<<$(echo E)"},
		{": <<a$(echo E)b\nbody\nEND\n", "<<a$(echo E)b"},
		{": <<\"$(echo E)\"\nbody\nEND\n", "<<$(echo E)"},
	} {
		var se *Error
		_, err := Parse(c.src, withDelimiterSubstitutions(HeredocDelimiterRefusesACommandSubstitution))
		if err == nil || !errors.As(err, &se) {
			t.Fatalf("%q = %v, want a refusal", c.src, err)
		}
		if se.Kind != ErrHeredocOutsideSubstitution {
			t.Errorf("%q kind = %v, want ErrHeredocOutsideSubstitution", c.src, se.Kind)
		}
		if se.Token != c.token {
			t.Errorf("%q token = %q, want %q", c.src, se.Token, c.token)
		}
		if got := int(se.Pos.Line); got != 1 {
			t.Errorf("%q line = %d, want 1 — the operator's own", c.src, got)
		}
	}
}

// The reading that scans nothing unquoted refuses the `(` where no word may
// have one, which is the sentence that grammar says about a `(` anywhere else
// rather than one of this question's own.
func TestAnUnscannedDelimiterSubstitutionRefusesItsParenthesis(t *testing.T) {
	t.Parallel()
	var se *Error
	src := ": <<$(echo E)\nbody\nEND\n"
	_, err := Parse(src, withDelimiterSubstitutions(HeredocDelimiterScansNoUnquotedSubstitution))
	if err == nil || !errors.As(err, &se) {
		t.Fatalf("%q = %v, want a refusal", src, err)
	}
	if se.Token != "(" {
		t.Errorf("token = %q, want %q", se.Token, "(")
	}
	if got := int(se.Pos.Line); got != 1 {
		t.Errorf("line = %d, want 1 — the delimiter's own", got)
	}
}

// And the control under all of it: a delimiter is not expanded under any
// reading, so the word the body is closed by is the one that was written.
func TestADelimiterIsNeverExpandedUnderAnyReading(t *testing.T) {
	t.Parallel()
	for _, a := range []HeredocDelimiterSubstitution{
		HeredocDelimiterTakesEverySubstitution,
		HeredocDelimiterScansNoUnquotedSubstitution,
		HeredocDelimiterRefusesACommandSubstitution,
	} {
		src := ": <<${v}\nbody\n${v}\necho after\n"
		f, err := Parse(src, withDelimiterSubstitutions(a))
		if err != nil {
			t.Fatalf("%v: %q = %v, want it read", a, src, err)
		}
		// Two commands, which is what says the document ended at the line
		// spelled like the delimiter rather than running to the input's end.
		if got := len(f.Stmts); got != 2 {
			t.Errorf("%v: %d statements, want 2", a, got)
		}
	}
}
