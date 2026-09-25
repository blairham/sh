// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// Dialect.ConditionIsResolvedWhenItRuns, both answers, over the arity half.
//
// The flag decides whether a known conditional operator standing with the
// wrong number of operands is a parse failure or a tree the interpreter
// refuses. Every row is written twice for that reason: the point is not what
// the sentence says, it is whether there is a sentence here at all. The other
// half of the flag — a name this dialect has no condition for — is the test
// below.
func TestAConditionsArityMayBeLeftForTheInterpreter(t *testing.T) {
	t.Parallel()
	arity := Core()
	arity.ConditionIsResolvedWhenItRuns = true
	for _, tc := range []struct {
		name, src string
		// op is the operator the accepted tree names, and words how many
		// stood with it. Empty op means the line is a parse failure under
		// both answers.
		op    string
		words int
	}{
		{"a surplus operand", `[[ -n x y ]]`, "-n", 2},
		{"two surplus operands", `[[ -n x -z "" ]]`, "-n", 3},
		{"no operand at all", `[[ -n ]]`, "-n", 0},
		{"another operator", `[[ -o ]]`, "-o", 0},
		{"three surplus operands", `[[ -n x y z ]]`, "-n", 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// With the flag on, the condition parses and the tree says which
			// operator had the wrong arity.
			f, err := Parse(tc.src, arity)
			if err != nil {
				t.Fatalf("with the flag on, parse %q: %v", tc.src, err)
			}
			got := testClauseOf(t, f)
			x, isArity := got.(*CondUnknown)
			if !isArity {
				t.Fatalf("with the flag on, %q parsed to %T, want *CondUnknown", tc.src, got)
			}
			if x.Op != tc.op {
				t.Errorf("the tree names %q, want %q", x.Op, tc.op)
			}
			if len(x.Words) != tc.words {
				t.Errorf("the tree kept %d words, want %d", len(x.Words), tc.words)
			}
			// And with it off, the same text is refused while reading, which
			// is what every other column in the panel does.
			if _, err := Parse(tc.src, Core()); err == nil {
				t.Errorf("with the flag off, %q parsed — it is a syntax error everywhere else", tc.src)
			}
		})
	}
}

// The rows the flag does **not** reach, which are what keep it from being
// "any word after the operand".
func TestAConditionsArityStopsWhereItWasMeasuredTo(t *testing.T) {
	t.Parallel()
	d := Core()
	d.ConditionIsResolvedWhenItRuns = true
	for _, tc := range []struct {
		name, src string
		refused   bool
	}{
		// An operator-shaped operand starts a reading of its own, so the
		// word after it is unexpected rather than surplus. Measured on the
		// shell this flag is for: `[[ -n -z x ]]` is a parse error naming
		// `x`, where `[[ -n x y ]]` is the run-time refusal.
		{"an operator-shaped operand leaves the next word unexpected", `[[ -n -z x ]]`, true},
		// And with nothing after it, that operand is an ordinary word.
		{"an operator-shaped operand on its own is an operand", `[[ -n -n ]]`, false},
		// A word that is no operator at all is a bare-word test, whatever
		// stands after it — the flag is about a *known* operator's arity.
		{"a word that is not an operator", `[[ -bogus ]]`, false},
		{"the ordinary arity", `[[ -n x ]]`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.src, d)
			if tc.refused != (err != nil) {
				t.Errorf("parse %q: err = %v, want refused = %v", tc.src, err, tc.refused)
			}
		})
	}
}

// testClauseOf digs the condition out of a one-statement file.
func testClauseOf(t *testing.T, f *File) CondExpr {
	t.Helper()
	if len(f.Stmts) != 1 {
		t.Fatalf("%d statements, want 1", len(f.Stmts))
	}
	tc, ok := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*TestClause)
	if !ok {
		t.Fatalf("%q did not parse to a test clause", f.Stmts[0].Expr)
	}
	return tc.Expr
}

// The same flag over the name half: a `-word` this dialect has no condition
// for at all parses, and the tree names it for the interpreter to refuse.
//
// #4261. `-Q` is the control — no dialect implements it — so what these rows
// are about is the spelling and not one missing operator.
func TestAConditionsNameMayBeLeftForTheInterpreter(t *testing.T) {
	t.Parallel()
	named := Core()
	named.ConditionIsResolvedWhenItRuns = true
	for _, tc := range []struct {
		name, src string
		op        string
		words     int
		// aWordWithoutTheFlag is a line that parses under both answers,
		// because with the flag off the word is an ordinary one: a single
		// word is a test for non-emptiness wherever it is not an operator.
		aWordWithoutTheFlag bool
	}{
		{name: "a letter nothing has", src: `[[ -Q x ]]`, op: "-Q", words: 1},
		{name: "a letter another dialect has", src: `[[ -R x ]]`, op: "-R", words: 1},
		{name: "a name rather than a letter", src: `[[ -bogus x ]]`, op: "-bogus", words: 1},
		{name: "a two-operand operator in front", src: `[[ -eq x ]]`, op: "-eq", words: 1},
		{name: "two dashes", src: `[[ -- x ]]`, op: "--", words: 1},
		{name: "two operands", src: `[[ -Q x y ]]`, op: "-Q", words: 2},
		{
			name: "no operand at all", src: `[[ -Q ]]`, op: "-Q", words: 0,
			aWordWithoutTheFlag: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := Parse(tc.src, named)
			if err != nil {
				t.Fatalf("with the flag on, parse %q: %v", tc.src, err)
			}
			got := testClauseOf(t, f)
			x, isUnknown := got.(*CondUnknown)
			if !isUnknown {
				t.Fatalf("with the flag on, %q parsed to %T, want *CondUnknown", tc.src, got)
			}
			if x.Op != tc.op {
				t.Errorf("the tree names %q, want %q", x.Op, tc.op)
			}
			if len(x.Words) != tc.words {
				t.Errorf("the tree kept %d words, want %d", len(x.Words), tc.words)
			}
			_, err = Parse(tc.src, Core())
			if tc.aWordWithoutTheFlag {
				if err != nil {
					t.Errorf("with the flag off, %q is one word and a test for non-emptiness: %v", tc.src, err)
				}
				return
			}
			if err == nil {
				t.Errorf("with the flag off, %q parsed — it is a syntax error everywhere else", tc.src)
			}
		})
	}
}

// The name half's boundaries, each a row a rule firing on every `-word` gets
// wrong. Measured on zsh 5.9.2, 2026-09-22.
func TestANameThatIsNoOperatorIsStillAWord(t *testing.T) {
	t.Parallel()
	d := Core()
	d.ConditionIsResolvedWhenItRuns = true
	for _, tc := range []struct {
		name, src string
		// want is the node the first primary parses to, or nil where the line
		// is refused while reading.
		want    string
		refused bool
	}{
		// Three characters or more and nothing behind it: the bare-word
		// reading, which is what `[[ -bogus ]]` is in that shell.
		{name: "a long name alone", src: `[[ -zz ]]`, want: "-n"},
		{name: "a long name before a connective", src: `[[ -zz && -n x ]]`, want: "-n"},
		{name: "a long name before a group's end", src: `[[ ( -zz ) ]]`, want: "-n"},
		// A two-operand operator behind the word makes the word its left
		// operand, whatever the word's length.
		{name: "a comparison", src: `[[ -Q == bar ]]`, want: "=="},
		{name: "arithmetic on a negative number", src: `[[ -1 -lt 2 ]]`, want: "-lt"},
		{name: "a string comparison", src: `[[ -Q < b ]]`, want: "<"},
		// And two shapes that are no name at all.
		{name: "a lone dash", src: `[[ - x ]]`, refused: true},
		{name: "a quoted name", src: `[[ "-Q" x ]]`, refused: true},
		// An operator-shaped operand starts a reading of its own here too, so
		// the word behind it is unexpected rather than surplus.
		{name: "an operator-shaped operand", src: `[[ -Q -n x ]]`, refused: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := Parse(tc.src, d)
			if tc.refused {
				if err == nil {
					t.Errorf("parse %q: accepted, want the ordinary token refusal", tc.src)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse %q: %v", tc.src, err)
			}
			switch x := firstPrimaryOf(t, f).(type) {
			case *CondUnary:
				if x.Op != tc.want {
					t.Errorf("%q read as %q, want %q", tc.src, x.Op, tc.want)
				}
			case *CondBinary:
				if x.Op != tc.want {
					t.Errorf("%q read as %q, want %q", tc.src, x.Op, tc.want)
				}
			default:
				t.Errorf("%q parsed to %T, want the word reading", tc.src, x)
			}
		})
	}
}

// firstPrimaryOf is the leftmost primary of a condition, with a connective or
// a group unwrapped — which is where a boundary row's reading is.
func firstPrimaryOf(t *testing.T, f *File) CondExpr {
	t.Helper()
	x := testClauseOf(t, f)
	for {
		switch n := x.(type) {
		case *CondLogic:
			x = n.X
		case *CondGroup:
			x = n.X
		case *CondNot:
			x = n.X
		default:
			return x
		}
	}
}

// The same flag over the **infix** half: a `-word` this dialect has no
// condition for, written between two operands, parses and the tree names it.
//
// #4437. The two positions are one rule in the dialect that has the flag and
// were two here — a name in front of its operand parsed and the same name
// between two operands was a syntax error — which is how a file the reference
// shell's own `-n` reads came to be refused by this parser's static read.
//
// [CondUnknown.Left] is checked on every row, because writing the left operand
// into Words would put the tree back in the wrong order and a printer would
// move the operator in front of a line somebody wrote infix.
func TestAnInfixConditionsNameMayBeLeftForTheInterpreter(t *testing.T) {
	t.Parallel()
	named := Core()
	named.ConditionIsResolvedWhenItRuns = true
	for _, tc := range []struct{ name, src, op string }{
		{"a name nothing implements", `[[ x -zzz y ]]`, "-zzz"},
		{"the shortest name that is one", `[[ x -zz y ]]`, "-zz"},
		{"a name a module would add", `[[ x -pcre-match y ]]`, "-pcre-match"},
		// A completion condition is a condition only in front of its
		// operands; written infix it is a name like any other, which is the
		// one place this parts from the prefix reading.
		{"a completion condition written infix", `[[ x -prefix y ]]`, "-prefix"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := named
			d.CompletionConditions = true
			f, err := Parse(tc.src, d)
			if err != nil {
				t.Fatalf("with the flag on, parse %q: %v", tc.src, err)
			}
			got := testClauseOf(t, f)
			x, isUnknown := got.(*CondUnknown)
			if !isUnknown {
				t.Fatalf("with the flag on, %q parsed to %T, want *CondUnknown", tc.src, got)
			}
			if x.Op != tc.op {
				t.Errorf("the tree names %q, want %q", x.Op, tc.op)
			}
			if x.Left == nil {
				t.Fatalf("%q kept no left operand — an infix operand written in "+
					"front of the operator belongs on Left, not in Words", tc.src)
			}
			if got, want := PrintWord(x.Left), "x"; got != want {
				t.Errorf("the left operand is %q, want %q", got, want)
			}
			if len(x.Words) != 1 {
				t.Fatalf("the tree kept %d words after the operator, want 1", len(x.Words))
			}
			if got, want := PrintWord(x.Words[0]), "y"; got != want {
				t.Errorf("the right operand is %q, want %q", got, want)
			}
			// And with the flag off the same text is refused while reading,
			// which is what every other column in the panel does.
			if _, err := Parse(tc.src, Core()); err == nil {
				t.Errorf("with the flag off, %q parsed — it is a syntax error everywhere else", tc.src)
			}
		})
	}
}

// The infix half's boundaries: each row holds one thing fixed and is what a
// rule reading "a `-word` between two words is an operator" gets wrong.
func TestAnInfixConditionsNameStopsWhereItWasMeasuredTo(t *testing.T) {
	t.Parallel()
	d := Core()
	d.ConditionIsResolvedWhenItRuns = true
	for _, tc := range []struct {
		name, src string
		refused   bool
	}{
		// Nothing behind the name, so it is no infix operator: the group
		// refusal, not this one.
		{"a name with no operand behind it", `[[ x -zz ]]`, true},
		// A one-operand test this dialect **has** keeps its own arity
		// wherever it stands.
		{"a one-operand test written infix", `[[ x -z y ]]`, true},
		// A two-operand operator this dialect has is read as itself, which
		// is what keeps a negative number on the left of a comparison out of
		// the refusal.
		{"a comparison this dialect has", `[[ -1 -lt 2 ]]`, false},
		// A quoted word is no operator: the reading is of the word as it was
		// written.
		{"a quoted name", `[[ x "-zzz" y ]]`, true},
		// And two characters is not a name — every one-operand test is
		// spelled that way, so the shortest infix name is three.
		{"a two-character name", `[[ x -q y ]]`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.src, d)
			if tc.refused != (err != nil) {
				t.Errorf("parse %q: err = %v, want refused = %v", tc.src, err, tc.refused)
			}
		})
	}
}
