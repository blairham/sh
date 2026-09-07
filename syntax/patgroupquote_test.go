// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// patGroup is the core with the two flags this file needs: bare groups, and
// the numeric range whose `<` a group has to be told apart from.
func patGroup() Dialect {
	d := Core()
	d.PatternAlternation = true
	d.NumericRangePattern = true
	// A `(` where an argument may stand belongs to the word, which is the
	// route that reaches the scanner with the group *opening* one — the
	// position the four operators are measured at.
	d.GlobQualifiers = true
	return d
}

// A quoted or escaped character inside a pattern group is text, not the
// operator it would be bare (#1248).
//
// The group used to be copied out of the source as raw bytes, so the scan
// stopped at the `<` however it was written and left the group's `)` to the
// parser. Every spelling of the protection is asserted, because they are
// three lexer paths — a backslash, single quotes and double quotes — and the
// bug was in the one place they all had to pass.
func TestAQuotedOperatorInsideAPatternGroupIsText(t *testing.T) {
	on := patGroup()
	for _, src := range []string{
		// The filed shape, at the front of a word so the group opens it.
		`[[ $k == (\<)* ]]`,
		`[[ $k == ("<")* ]]`,
		`[[ $k == ('<')* ]]`,
		// The other three operators that end a word where a group starts
		// one, which is the same clause and so the same bug.
		`[[ $k == (\>)* ]]`,
		`[[ $k == (a\;b)* ]]`,
		`[[ $k == (a\&b)* ]]`,
		`[[ $k == ("a;b")* ]]`,
		`[[ $k == ("a&b")* ]]`,
		// The group's own delimiters, which a backslash also takes away.
		`[[ $k == (a\)b)* ]]`,
		`[[ $k == (a\(b)* ]]`,
		// An alternation with a protected operator in one arm only.
		`[[ $k == (\<|q)* ]]`,
		// Nested, and inside a range's own delimiters.
		`[[ $k == ((\<))* ]]`,
		`[[ $k == (\<0-9\>)* ]]`,
		// Mid-word, which is the other route into the scanner: the group
		// does not open the word there, and the protection is what carries
		// the operator through — see the refusals below, where the same
		// group without it is a parse error.
		`[[ $k == a(\<)b ]]`,
		`[[ $k == a(b\<c) ]]`,
		`[[ $k == a("b;c") ]]`,
		// An ordinary argument, which is the other way a word can carry a
		// group that opens it.
		`echo (\<)*`,
	} {
		if _, err := Parse(src, on); err != nil {
			t.Errorf("%s: %v", src, err)
		}
	}
	// Bare, the four are still operators and still end the word where the
	// group opens it — which is what says the fix is about quoting rather
	// than about giving the group the four characters. Measured on zsh
	// 5.9.2: each of these is a parse error there.
	//
	// And a group in the *middle* of a word ends it at the same four, which
	// is neither the route nor the position the rule was written as: reading
	// them into the group matched `ab<c` against `a(b<c)` where zsh 5.9.2
	// will not read the line at all.
	for _, src := range []string{
		`[[ $k == (<)* ]]`,
		`[[ $k == (>)* ]]`,
		`[[ $k == (a;b)* ]]`,
		`[[ $k == (a&b)* ]]`,
		`[[ $k == a(b<c) ]]`,
		`[[ $k == a(b>c) ]]`,
		`[[ $k == a(b;c) ]]`,
		`[[ $k == a(b&c) ]]`,
		`case $k in a(b<c)) echo m;; esac`,
	} {
		if _, err := Parse(src, on); err == nil {
			t.Errorf("%s: parsed; the bare operator still ends the word", src)
		}
	}
	// A `|` is the group's wherever the group stands, which is the control
	// for the rows above: it is the one of the five that does not end a word.
	for _, src := range []string{`[[ $k == a(b|c) ]]`, `[[ $k == a(<0-9>) ]]`} {
		if _, err := Parse(src, on); err != nil {
			t.Errorf("%s: %v", src, err)
		}
	}
	// A line continuation is removed before tokens are formed, inside a
	// group as everywhere else, so it can split one anywhere and leaves
	// nothing behind. Written out because the backslash case beside it does
	// the opposite — it *keeps* the byte it protects — and reading the
	// newline as protected would make `(a\<newline>b)` the pattern `a`,
	// newline, `b`, which matches `ab` not at all.
	f, err := Parse("echo (a\\\nb)", on)
	if err != nil {
		t.Fatalf("a line continuation inside a group: %v", err)
	}
	cmd := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
	var text string
	for _, sp := range cmd.Args[1].Spans {
		text += sp.Value
	}
	if text != "(ab)" {
		t.Errorf("a line continuation inside a group left %q, want %q", text, "(ab)")
	}
	// And a bare numeric range is still taken whole, which is the flag the
	// `<` cases have to stay out of the way of (#1217).
	if _, err := Parse(`[[ $k == (<0-9>)* ]]`, on); err != nil {
		t.Errorf("a bare numeric range inside a group: %v", err)
	}
}

// The quoting has to survive into the tree, not only get past the parser.
//
// This is the half that would have been silent: a group scanned as one
// unquoted literal parses perfectly and hands the matcher a `"` to match, so
// `[[ b == ("b") ]]` asked whether `b` is three characters and said no.
func TestAPatternGroupKeepsTheQuotingInsideIt(t *testing.T) {
	on := patGroup()
	for _, tc := range []struct {
		src   string
		value string // the span the protection produced
		quote Quoting
	}{
		{`echo (\<)`, "<", BackslashQuoted},
		{`echo ("<")`, "<", DoubleQuoted},
		{`echo ('<')`, "<", SingleQuoted},
		{`echo a("b")c`, "b", DoubleQuoted},
	} {
		f, err := Parse(tc.src, on)
		if err != nil {
			t.Errorf("%s: %v", tc.src, err)
			continue
		}
		cmd, ok := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
		if !ok {
			t.Errorf("%s: not a simple command", tc.src)
			continue
		}
		var found bool
		for _, s := range cmd.Args[1].Spans {
			if s.Quoting == tc.quote && s.Value == tc.value {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: no %v span holding %q — the group was taken as text: %#v",
				tc.src, tc.quote, tc.value, cmd.Args[1].Spans)
		}
	}
}

// Printed source has to mean the same thing, which for a group means the
// quoting comes back: printing `(<)` where `(\<)` was written is a parse
// error, and printing `(b)` where `("b")` was written is a different pattern.
func TestAQuotedPatternGroupPrintsBackAsItWasWritten(t *testing.T) {
	on := patGroup()
	for _, src := range []string{
		`echo (\<)*`,
		`echo ("<")*`,
		`echo ('<')*`,
		`echo (a\;b)`,
		`echo a("b")c`,
	} {
		f, err := Parse(src, on)
		if err != nil {
			t.Errorf("%s: %v", src, err)
			continue
		}
		printed := Print(f)
		g, err := Parse(printed, on)
		if err != nil {
			t.Errorf("%s: printed as %q, which does not parse: %v", src, printed, err)
			continue
		}
		if again := Print(g); again != printed {
			t.Errorf("%s: printed %q, reprinted %q", src, printed, again)
		}
	}
}
